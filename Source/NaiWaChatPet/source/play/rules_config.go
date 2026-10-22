package play

import (
	"encoding/json"
	"os"
)

func ReadCombatRules(filename string) (CombatRules, error) {
	b, e := os.ReadFile(filename)
	if e != nil {
		return CombatRules{}, e
	}
	return ParseCombatRules(b)
}

// Configured numbers are validated. Existing authored animation events and
// durations still take precedence over sidecar fallback timeline values.
func NewConfiguredCombat(challenge bool, rules *CombatRules) (*Combat, error) {
	c := NewCombat(challenge)
	if rules == nil {
		return c, nil
	}
	encoded, e := json.Marshal(rules)
	if e != nil {
		return nil, e
	}
	r, e := ParseCombatRules(encoded)
	if e != nil {
		return nil, e
	}
	for i := range r.Skills {
		if r.Skills[i].Clip == c.Rules.Skills[i].Clip && r.Skills[i].Clip != "" {
			r.Skills[i].Duration = c.Rules.Skills[i].Duration
			if c.library == nil {
				continue
			}
			for _, event := range c.library.GetAnimationEvents(r.Skills[i].Clip) {
				switch event.Name {
				case "fan_release", "roll_start", "power_apply":
					r.Skills[i].Effect = c.Rules.Skills[i].Effect
				case "roll_land":
					r.Skills[i].ActiveEnd = c.Rules.Skills[i].ActiveEnd
				}
			}
		}
	}
	c.Rules = r
	c.Rules.Skills[DodgeSkill-1].Cooldown = 0
	return c, nil
}
