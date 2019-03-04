package channels

import (
	"sync"

	"github.com/bspaans/bs8bs/audio"
	"github.com/bspaans/bs8bs/generators"
	"github.com/bspaans/bs8bs/instruments"
	"github.com/bspaans/bs8bs/midi/notes"
)

type PercussionChannel struct {
	On          *sync.Map
	Instruments []generators.Generator
	FX          ChannelFX
}

func NewPercussionChannel() *PercussionChannel {
	p := &PercussionChannel{
		On: &sync.Map{},
	}
	p.LoadInstrumentsFromBank()
	return p
}

func (c *PercussionChannel) LoadInstrumentsFromBank() {
	instr := make([]generators.Generator, 128)
	for i, gen := range instruments.Banks[1] {
		if gen != nil {
			instr[i] = gen()
		}
	}
	c.Instruments = instr
}

func (c *PercussionChannel) getInstrument(note int) generators.Generator {
	return c.Instruments[note]
}

func (c *PercussionChannel) SetInstrument(g func() generators.Generator) {
}

func (c *PercussionChannel) NoteOn(note int, velocity float64) {
	instr := c.getInstrument(note)
	if instr != nil {
		instr.SetPitch(notes.NoteToPitch[note])
		instr.SetGain(velocity)
		c.On.Store(note, true)
	}
}

func (c *PercussionChannel) NoteOff(note int) {
	instr := c.getInstrument(note)
	if instr != nil {
		instr.SetPitch(0.0)
		c.On.Delete(note)
	}
}

func (c *PercussionChannel) SetPitchbend(f float64) {
}

func (c *PercussionChannel) GetSamples(cfg *audio.AudioConfig, n int) []float64 {
	result := generators.GetEmptySampleArray(cfg, n)
	c.On.Range(func(on, value interface{}) bool {
		for i, s := range c.Instruments[on.(int)].GetSamples(cfg, n) {
			result[i] += s
		}
		return true
	})
	filter := c.FX.Filter()
	if filter == nil {
		return result
	}
	return filter.Filter(cfg, result)
}

func (c *PercussionChannel) SetFX(fx FX, value float64) {
	c.FX.Set(fx, value)
}
