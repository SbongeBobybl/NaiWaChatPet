package rig

import (
	"embed"
	"fmt"
	"sync"
)

// Original user files copied byte-for-byte from naiwa-skills-skeletal.tar.gz.
//
//go:embed skilldata/naiwa.json skilldata/naiwa.atlas skilldata/images/*.png
var skillAssets embed.FS
var skillOnce sync.Once
var skillLibrary *SkillAnimations
var skillError error

var skillOverrideMutex sync.RWMutex
var skillOverride *SkillAnimations

func RegisterSkillAnimations(library *SkillAnimations) error {
	if library == nil {
		return fmt.Errorf("nil skill library")
	}
	for _, name := range []string{"skill_fan_throw", "skill_roll", "skill_power_up"} {
		if library.GetAnimationDuration(name) <= 0 {
			return fmt.Errorf("missing required user animation %s", name)
		}
	}
	skillOverrideMutex.Lock()
	skillOverride = library
	skillOverrideMutex.Unlock()
	return nil
}

func DefaultSkillAnimations() (*SkillAnimations, error) {
	skillOverrideMutex.RLock()
	override := skillOverride
	skillOverrideMutex.RUnlock()
	if override != nil {
		return override, nil
	}
	skillOnce.Do(func() { skillLibrary, skillError = LoadSkillAnimations(skillAssets, "skilldata/naiwa.json") })
	return skillLibrary, skillError
}
