package monitor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"sync"

	"github.com/UniversityRadioYork/alapi/pkg/config"
	soundio "github.com/crow-misia/go-libsoundio"
)

var prioritizedFormats = []soundio.Format{
	soundio.FormatFloat64NE,
	soundio.FormatFloat64FE,
	soundio.FormatFloat32NE,
	soundio.FormatFloat32FE,
}

var prioritizedSampleRates = []int{
	48000,
	44100,
	96000,
	24000,
}

type DevicesMonitor struct {
	Config     *config.Config
	device     *soundio.Device
	stream     *soundio.InStream
	level      float64
	overflow   uint64
	levelsLock sync.RWMutex
}

func (d *DevicesMonitor) Init(device *soundio.Device) error {
	if err := device.ProbeError(); err != nil {
		return err
	}
	device.SortChannelLayouts()
	sampleRate := 0
	for _, rate := range prioritizedSampleRates {
		if device.SupportsSampleRate(rate) {
			sampleRate = rate
			break
		}
	}
	if sampleRate == 0 {
		sampleRate = device.SampleRates()[0].Max()
	}
	log.Printf("Device %s: sample rate: %d", device.ID(), sampleRate)

	format := soundio.FormatInvalid
	for _, f := range prioritizedFormats {
		if device.SupportsFormat(f) {
			format = f
			break
		}
	}
	if format == soundio.FormatInvalid {
		format = device.Formats()[0]
	}
	log.Printf("Device %s: format: %s", device.ID(), format)

	stream, err := device.NewInStream(&soundio.InStreamConfig{
		Format:     format,
		SampleRate: sampleRate,
	})
	if err != nil {
		return err
	}
	stream.SetReadCallback(d.readCallback)
	stream.SetOverflowCallback(d.overflowCallback)
	d.device = device
	d.stream = stream
	d.level = 0
	if err := d.stream.Start(); err != nil {
		log.Println(err)
	}
	return nil
}

func (d *DevicesMonitor) overflowCallback(stream *soundio.InStream) {
	d.overflow++
	log.Printf("device %s; overflow %d", stream.Device().ID(), d.overflow)
}

func (d *DevicesMonitor) readCallback(stream *soundio.InStream, frameCountMin int, frameCountMax int) {
	format := stream.Format()
	channels := stream.Layout()
	bufferLength := int(math.Ceil(float64(soundio.BytesPerSecond(format, channels.ChannelCount(), stream.SampleRate())) * d.Config.BufferLength))
	frameBytes := stream.Layout().ChannelCount() * stream.BytesPerFrame()
	framesLeft := bufferLength / frameBytes
	for {
		frameCount := framesLeft
		if frameCount <= 0 {
			break
		}
		// log.Printf("reading %d frames", frameCount)
		areas, err := stream.BeginRead(&frameCount)
		if err != nil {
			log.Printf("beginRead: %v", err)
			return
		}
		if frameCount <= 0 {
			break
		}
		if areas != nil {
			var sumSquares float64
			var n int
			for frame := 0; frame < frameCount; frame++ {
				for ch := 0; ch < channels.ChannelCount(); ch++ {
					buffer := areas.Buffer(ch, frame)
					float, err := convertBuffer(stream.Format(), bytes.NewReader(buffer))
					if err != nil {
						log.Printf("convertBuffer: %v", err)
						return
					}
					// log.Printf("read %v", buffer)

					sumSquares += (float * float)
					n++
				}
			}
			rms := math.Sqrt(sumSquares / float64(n))
			d.levelsLock.Lock()
			d.level = rms
			d.levelsLock.Unlock()
		}
		err = stream.EndRead()
		if err != nil {
			log.Printf("endRead: %v", err)
			return
		}
		framesLeft -= frameCount
	}
}

const rangeS16 = 32767
const rangeS32 = 65535

func convertBuffer(format soundio.Format, buffer io.Reader) (float64, error) {
	var order binary.ByteOrder
	switch format {
	case soundio.FormatFloat32BE, soundio.FormatFloat64BE, soundio.FormatS16BE, soundio.FormatS32BE:
		order = binary.BigEndian
	case soundio.FormatFloat32LE, soundio.FormatFloat64LE, soundio.FormatS16LE, soundio.FormatS32LE:
		order = binary.LittleEndian
	default:
		return 0, fmt.Errorf("unsupported format %#v", format)
	}
	var read float64
	var err error
	switch format {
	case soundio.FormatFloat32BE, soundio.FormatFloat32LE:
		var val float32
		err = binary.Read(buffer, order, &val)
		read = float64(val)
	case soundio.FormatFloat64BE, soundio.FormatFloat64LE:
		err = binary.Read(buffer, order, &read)
	case soundio.FormatS16BE, soundio.FormatS16LE:
		var val int16
		err = binary.Read(buffer, order, &val)
		read = float64(val) / rangeS16
	case soundio.FormatS32LE, soundio.FormatS32BE:
		var val int32
		err = binary.Read(buffer, order, &val)
		read = float64(val) / rangeS32
	}

	if err != nil {
		return 0, err
	}
	return read, nil
}

func (d *DevicesMonitor) GetLevels() float64 {
	d.levelsLock.RLock()
	val := d.level
	d.levelsLock.RUnlock()
	return val
}

func (d *DevicesMonitor) Close() error {
	d.stream.Destroy()
	return nil
}
