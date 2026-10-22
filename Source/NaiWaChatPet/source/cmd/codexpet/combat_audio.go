package main

import (
	"encoding/binary"
	"math"
)

// Original procedural PCM effects, no external sound sample licenses needed.
func combatWAV(kind string, volume float64) []byte {
	const rate = 22050
	duration := .18
	if kind == "explosion" {
		duration = .55
	}
	if kind == "power" {
		duration = .45
	}
	count := int(duration * rate)
	b := make([]byte, 44+count*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[28:], rate*2)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(count*2))
	seed := uint32(8731)
	for i := 0; i < count; i++ {
		t := float64(i) / rate
		seed = seed*1664525 + 1013904223
		noise := float64(seed>>8)/float64(1<<24)*2 - 1
		env := math.Exp(-t * 25)
		sample := noise*env*.6 + math.Sin(2*math.Pi*(130*t-80*t*t))*env*.4
		switch kind {
		case "explosion":
			sample = .42*noise*math.Exp(-t*10) + .45*math.Sin(2*math.Pi*(75*t-35*t*t))*math.Exp(-t*8) + .1*noise*math.Sin(t*350)*math.Exp(-t*5)
		case "power":
			sample = (math.Sin(2*math.Pi*440*t) + .5*math.Sin(2*math.Pi*660*t) + .3*math.Sin(2*math.Pi*880*t)) * math.Sin(math.Pi*t/duration) * math.Exp(-t*4) * .4
		case "dodge":
			sample = noise * math.Sin(math.Pi*t/duration) * math.Exp(-t*10) * .35
		case "roll":
			sample = noise*math.Exp(-t*35)*.6 + math.Sin(2*math.Pi*95*t)*env*.5
		}
		sample = math.Max(-1, math.Min(1, sample*volume))
		binary.LittleEndian.PutUint16(b[44+i*2:], uint16(int16(sample*28000)))
	}
	return b
}
