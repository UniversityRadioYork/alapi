extern crate anyhow;
extern crate cpal;
extern crate crossbeam;
extern crate rouille;

mod config;

use anyhow::Result;
use ringbuf::RingBuffer;
use rouille::Response;
use std::collections::HashMap;
use std::env::args;
use std::sync::{Arc, Mutex};
use std::time::Duration;
use cpal::{Device, Stream, StreamConfig};
use cpal::traits::{HostTrait, DeviceTrait, StreamTrait};
use crossbeam::channel::{Sender, bounded, TrySendError};
use crate::config::Config;

struct Monitor {
    _stream: Stream
}

impl Monitor {
    pub(crate) fn create(device: Device, period: Duration, channel_map: &'static HashMap<usize, String>, results: Sender<HashMap<String, f32>>) -> Result<Self> {
        for cfg in device.supported_input_configs()? {
            println!("for device {}: supported but non-default input config {:?}", device.name()?, cfg);
        }
        let cfg = device.default_input_config()?;
        println!("for device {} using input config {:?}", device.name()?, cfg);

        let err_fn = move |err| {
            eprintln!("an error occurred on stream: {}", err);
        };

        let stream_config: Arc<StreamConfig> = Arc::new(cfg.clone().into());
        println!("Got {} channels", stream_config.channels);

        let samples_per_period: usize = (stream_config.sample_rate.0 as f32 * period.as_secs_f32()) as usize;
        println!("Samples per period: {}", samples_per_period);

        let mut channel_buffers = {
            let mut bufs: HashMap<usize, (ringbuf::Producer<f32>, ringbuf::Consumer<f32>)> = HashMap::with_capacity(channel_map.len());
            for key in channel_map.keys() {
                bufs.insert(*key, RingBuffer::new(samples_per_period).split());
            }
            bufs
        };
        println!("set up channel_buffers with {} items", channel_buffers.len());

        let another_stream_config = Arc::clone(&stream_config);
        let stream = match cfg.sample_format() {
            cpal::SampleFormat::F32 => device.build_input_stream(
                &stream_config,
                move |data: &[f32], _: &_| {
                    let mut any_buf_full = false;
                    for (i, val) in data.iter().enumerate() {
                        // FIXME won't handle non-consecutive channels]
                        let channel = i % (another_stream_config.clone().channels as usize);
                        if let Some((prod, _)) = channel_buffers.get_mut(&channel) {
                            let _ = prod.push(*val);
                            if prod.is_full() {
                                any_buf_full = true;
                            }
                        } else {
                            println!("No matching channel for {}", channel);
                        }
                    }

                    if any_buf_full {
                        let mut result: HashMap<String, f32> = HashMap::with_capacity(channel_map.len());
                        for (channel, (_, cons)) in channel_buffers.iter_mut() {
                            let mut sum_of_squares = 0.0;
                            let mut count = 0;
                            cons.pop_each(|sample| {
                                sum_of_squares += sample * sample;
                                count += 1;
                                true
                            }, None);
                            let channel_name = &channel_map[&channel];
                            result.insert(channel_name.to_string(), (sum_of_squares / count as f32).sqrt());
                        }
                        match results.try_send(result) {
                            Ok(()) => {},
                            Err(TrySendError::Full(_)) => {},
                            Err(x) => panic!("{}",x)
                        }
                    }
                },
                err_fn,
            ),
            _ => panic!("no")
        }?;
        stream.play()?;

        Ok(Monitor{_stream: stream})
    }
}

fn main() -> anyhow::Result<()> {
    let config_path = args().nth(1).expect("Usage: alapi <config.toml>");
    let config = Config::load_config(config_path)?;
    println!("{:?}", config);

    let channel_map: HashMap<usize, String> = {
        let mut hm = HashMap::with_capacity(config.channel_map.len());
        for (key, val) in &config.channel_map {
            hm.insert(key.parse::<usize>().unwrap(), val.to_string());
        }
        hm
    };

    let hosts = cpal::available_hosts()
        .into_iter()
        .map(|id| id.name())
        .collect::<Vec<_>>()
        .join(", ");
    println!("Available hosts: {}", hosts);

    let host = cpal::host_from_id(
        cpal::available_hosts()
            .into_iter()
            .find(|id| id.name() == config.host)
            .expect("Could not find host!"),
    )?;

    host.input_devices()?.for_each(|dev| {
        println!("Available device: {}", dev.name().unwrap());
    });

    let device = host.input_devices()?.find(|d| {
        d.name().unwrap() == config.device
    }).expect("No matching device found");

    let (send, recv) = bounded::<HashMap<String, f32>>(0);

    let sampling_factor = config.sampling_factor;

    let _monitor = Monitor::create(device, Duration::from_nanos((1e9 / sampling_factor as f64) as u64), Box::leak(Box::new(channel_map)), send)?;

    let latest_values: Arc<Mutex<Vec<HashMap<String, f32>>>> = Arc::new(Mutex::new(Vec::new()));

    let lv1 = Arc::clone(&latest_values);
    std::thread::spawn(move || {
       loop {
           let val = recv.recv().unwrap();
           {
               let mut mush = lv1.lock().unwrap();
               mush.insert(0, val);
               mush.truncate(sampling_factor);
           }
       }
    });

    let lv2 = Arc::clone(&latest_values);
    rouille::start_server(config.http_bind, move |_| {
        let mut result: HashMap<String, f32> = HashMap::new();
        let data = lv2.lock().unwrap();
        for row in data.iter() {
            for (key, val) in row {
               *result.entry(key.to_string()).or_insert(0.0) += val;
            }
        }
        for (_, val) in result.iter_mut() {
            *val /= sampling_factor as f32;
        }
        Response::json(&result)
    })
}
