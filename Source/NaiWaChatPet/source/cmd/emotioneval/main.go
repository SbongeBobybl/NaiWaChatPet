package main

import (
	"codexpet/reconstruction/emotion"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Sample struct {
	ID          int    `json:"id"`
	User        string `json:"user"`
	Reply       string `json:"reply"`
	Stage       string `json:"stage"`
	Expected    string `json:"expected"`
	Split       string `json:"split"`
	LabelStatus string `json:"label_status"`
}

func main() {
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	var data []Sample
	if json.Unmarshal(b, &data) != nil {
		panic("dataset invalid")
	}
	labels := []string{"neutral", "happy", "shy", "surprised", "sad", "angry", "confused", "comforting"}
	matrix := map[string]map[string]int{}
	for _, a := range labels {
		matrix[a] = map[string]int{}
	}
	total, correct, falseHappy, predHappy := 0, 0, 0, 0
	latency := time.Duration(0)
	var errors []map[string]any
	for _, s := range data {
		if s.Split != "heldout" {
			continue
		}
		started := time.Now()
		d := emotion.Analyze(emotion.Cue{Stage: s.Stage, User: s.User, Reply: s.Reply})
		latency += time.Since(started)
		matrix[s.Expected][d.Emotion]++
		total++
		if d.Emotion == s.Expected {
			correct++
		} else {
			errors = append(errors, map[string]any{"id": s.ID, "expected": s.Expected, "predicted": d.Emotion})
		}
		if d.Emotion == "happy" {
			predHappy++
			if s.Expected != "happy" {
				falseHappy++
			}
		}
	}
	metrics := map[string]any{}
	macro := 0.0
	for _, l := range labels {
		tp := matrix[l][l]
		actual, pred := 0, 0
		for _, other := range labels {
			actual += matrix[l][other]
			pred += matrix[other][l]
		}
		p, r := 0., 0.
		if pred > 0 {
			p = float64(tp) / float64(pred)
		}
		if actual > 0 {
			r = float64(tp) / float64(actual)
		}
		f := 0.
		if p+r > 0 {
			f = 2 * p * r / (p + r)
		}
		macro += f
		metrics[l] = map[string]any{"precision": p, "recall": r, "f1": f, "support": actual}
	}
	result := map[string]any{"label_status": "assistant_draft_not_human_reviewed", "accepted_80_percent_target": false, "heldout_count": total, "accuracy": float64(correct) / float64(total), "macro_f1": macro / float64(len(labels)), "per_class": metrics, "confusion_matrix": matrix, "false_happy_count": falseHappy, "predicted_happy": predHappy, "mean_classifier_latency_ms": float64(latency.Microseconds()) / 1000 / float64(total), "errors": errors, "note": "Template scenario regression, not a human-labelled generalization claim. GUI scheduling latency is separate."}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	if len(os.Args) > 2 {
		os.WriteFile(os.Args[2], out, 0600)
	}
}
