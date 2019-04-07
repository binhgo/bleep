package sequencer

import (
	"fmt"
	"time"

	"github.com/bspaans/bleep/channels"
	"github.com/bspaans/bleep/synth"
)

type Sequence func(seq *Sequencer, counter, t uint, s chan *synth.Event)

func Whole(seq *Sequencer) uint {
	return uint(seq.Granularity) * 4
}
func Half(seq *Sequencer) uint {
	return uint(seq.Granularity) * 2
}

func Quarter(seq *Sequencer) uint {
	return uint(seq.Granularity)
}

func Eight(seq *Sequencer) uint {
	return uint(seq.Granularity / 2)
}
func Sixteenth(seq *Sequencer) uint {
	return uint(seq.Granularity / 4)
}
func Thirtysecond(seq *Sequencer) uint {
	return uint(seq.Granularity / 8)
}

type Sequencer struct {
	BPM                 float64
	Granularity         int
	Sequences           []Sequence
	Inputs              chan *SequencerEvent
	Time                uint
	FromFile            string
	Started             bool
	InitialChannelSetup []*channels.ChannelDef
	IntRegisters        []int
	IntArrayRegisters   [][]int
	FloatRegisters      []float64
}

func NewSequencer(bpm float64, granularity int) *Sequencer {
	seq := &Sequencer{
		BPM:               bpm,
		Granularity:       granularity,
		Sequences:         []Sequence{},
		Inputs:            make(chan *SequencerEvent, 32),
		IntRegisters:      make([]int, 128),
		IntArrayRegisters: make([][]int, 128),
		FloatRegisters:    make([]float64, 128),
	}
	return seq
}

func NewSequencerFromFile(file string) (*Sequencer, error) {
	s, err := NewSequencerDefFromFile(file)
	if err != nil {
		return nil, WrapError("sequencer", err)
	}
	seq := NewSequencer(s.BPM, s.Granularity)
	seqs, err := s.GetSequences(seq)
	if err != nil {
		return nil, WrapError("sequencer", err)
	}
	seq.Sequences = seqs
	seq.InitialChannelSetup = s.Channels.Channels
	seq.FromFile = file
	return seq, nil
}

func (seq *Sequencer) Start(s chan *synth.Event) {
	if seq.Started {
		fmt.Println("Sequencer already started")
		return
	}
	fmt.Println("Starting sequencer")
	seq.Started = true
	go seq.start(s)
}

func (seq *Sequencer) start(s chan *synth.Event) {

	seq.Time = uint(0)

	for {

		start := time.Now()
		if seq.Time == 0 {
			s <- synth.NewEvent(synth.SilenceAllChannels, 0, nil)
			seq.loadInstruments(s)
		}

		for _, sequence := range seq.Sequences {
			sequence(seq, seq.Time, seq.Time, s)
		}

		seq.Time += 1

		canRead := true
		for canRead {
			select {
			case ev := <-seq.Inputs:
				if ev.Type == QuitSequencer {
					fmt.Println("Quitting sequencer")
					return
				}
				seq.dispatchEvent(ev)
			default:
				canRead = false
			}
		}

		millisecondsPerBeat := 60000.0 / seq.BPM
		millisecondsPerTick := time.Duration(millisecondsPerBeat / float64(seq.Granularity) * 1000000)

		elapsed := time.Now().Sub(start)
		if elapsed > millisecondsPerTick {
			fmt.Println("Warning: sequencer underrun", millisecondsPerTick, elapsed)
		} else {
			sleep := millisecondsPerTick - elapsed
			time.Sleep(sleep)
		}
	}
}

func (seq *Sequencer) loadInstruments(s chan *synth.Event) {
	for _, channelDef := range seq.InitialChannelSetup {
		ch := channelDef.Channel
		if ch != 9 {
			if channelDef.Generator == nil {
				s <- synth.NewEvent(synth.ProgramChange, ch, []int{channelDef.Instrument})
			} else {
				if err := channelDef.Generator.Validate(); err != nil {
					fmt.Printf("Failed to load generator for channel %d; %s\n", ch, err.Error())
				} else {
					instr := channelDef.Generator.Generator
					s <- synth.NewInstrumentEvent(synth.SetInstrument, ch, instr)
				}
			}
		}
		s <- synth.NewEvent(synth.SetTremelo, ch, []int{channelDef.Tremelo})
		s <- synth.NewEvent(synth.SetReverb, ch, []int{channelDef.Reverb})
		s <- synth.NewEvent(synth.SetLPFCutoff, ch, []int{channelDef.LPF_Cutoff})
		s <- synth.NewEvent(synth.SetHPFCutoff, ch, []int{channelDef.HPF_Cutoff})
		s <- synth.NewEvent(synth.SetChannelVolume, ch, []int{channelDef.Volume})
		s <- synth.NewEvent(synth.SetChannelPanning, ch, []int{channelDef.Panning})
		s <- synth.NewFloatEvent(synth.SetReverbFeedback, ch, []float64{channelDef.ReverbFeedback})

		d, err := channels.ParseDuration(channelDef.ReverbTime, seq.BPM)
		if err == nil {
			s <- synth.NewFloatEvent(synth.SetReverbTime, ch, []float64{d})
		} else {
			fmt.Println("Invalid duration:", err.Error())
		}

		if channelDef.Grain != nil {
			g := channelDef.Grain
			s <- synth.NewStringEvent(synth.SetGrain, ch, g.File)
			s <- synth.NewFloatEvent(synth.SetGrainGain, ch, []float64{channelDef.Grain.Gain})
			s <- synth.NewFloatEvent(synth.SetGrainSize, ch, []float64{channelDef.Grain.GrainSize})
			s <- synth.NewFloatEvent(synth.SetGrainBirthRate, ch, []float64{channelDef.Grain.BirthRate})
			s <- synth.NewFloatEvent(synth.SetGrainSpread, ch, []float64{channelDef.Grain.Spread})
			s <- synth.NewFloatEvent(synth.SetGrainSpeed, ch, []float64{channelDef.Grain.Speed})
			s <- synth.NewEvent(synth.SetGrainDensity, ch, []int{channelDef.Grain.Density})
		}
	}
}

func (seq *Sequencer) dispatchEvent(ev *SequencerEvent) {
	if ev.Type == RestartSequencer {
		seq.Time = 0
	} else if ev.Type == ReloadSequencer {
		seq.Time = 0
		fmt.Println("reloading")
		if seq.FromFile != "" {
			s, err := NewSequencerDefFromFile(seq.FromFile)
			if err != nil {
				fmt.Println("Failed to reload sequencer:", err.Error())
				return
			}
			seq.BPM = s.BPM
			seq.Granularity = s.Granularity
			seq.InitialChannelSetup = s.Channels.Channels
			seqs, err := s.GetSequences(seq)
			if err != nil {
				fmt.Println("Failed to reload sequencer:", err.Error())
				return
			}
			seq.Sequences = seqs
		}
	} else if ev.Type == ForwardSequencer {
		seq.Time += uint(seq.Granularity) * 16
		fmt.Println("t =", seq.Time)
	} else if ev.Type == BackwardSequencer {
		if seq.Time < uint(seq.Granularity)*16 {
			seq.Time = 0
		} else {
			seq.Time -= uint(seq.Granularity) * 16
		}
		fmt.Println("t =", seq.Time)
	} else if ev.Type == IncreaseBPM {
		seq.BPM += 10
		fmt.Println("bpm =", seq.BPM)
	} else if ev.Type == DecreaseBPM {
		seq.BPM -= 10
		if seq.BPM <= 0 {
			seq.BPM = 1
		}
		fmt.Println("bpm =", seq.BPM)
	} else if ev.Type == QuitSequencer {
		seq.Time = 0
		return
	}
}

func (seq *Sequencer) Restart() {
	seq.Inputs <- NewSequencerEvent(RestartSequencer)
}
func (seq *Sequencer) Reload() {
	seq.Inputs <- NewSequencerEvent(ReloadSequencer)
}
func (seq *Sequencer) MoveForward() {
	seq.Inputs <- NewSequencerEvent(ForwardSequencer)
}
func (seq *Sequencer) MoveBackward() {
	seq.Inputs <- NewSequencerEvent(BackwardSequencer)
}
func (seq *Sequencer) IncreaseBPM() {
	seq.Inputs <- NewSequencerEvent(IncreaseBPM)
}
func (seq *Sequencer) DecreaseBPM() {
	seq.Inputs <- NewSequencerEvent(DecreaseBPM)
}
func (seq *Sequencer) Quit() {
	seq.Inputs <- NewSequencerEvent(QuitSequencer)
}
