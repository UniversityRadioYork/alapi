use anyhow::Result;
use serde_derive::Deserialize;
use std::collections::HashMap;
use std::fs::File;
use std::io::Read;

fn default_bind() -> String {
    "0.0.0.0".to_string()
}

const fn default_sampling_factor() -> usize {1}

#[derive(Deserialize, Debug)]
pub struct Config {
    pub host: String,
    pub device: String,
    pub min_channels: usize,
    pub channel_map: HashMap<String, String>,
    #[serde(default = "default_bind")]
    pub http_bind: String,
    #[serde(default = "default_sampling_factor")]
    pub sampling_factor: usize,
}

impl Config {
    pub fn load_config(path: String) -> Result<Self> {
        let mut file = File::open(path)?;
        let mut contents = String::new();
        file.read_to_string(&mut contents)?;
        let cfg = toml::from_str::<Config>(&contents)?;
        return Ok(cfg);
    }
}
