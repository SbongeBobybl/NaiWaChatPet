package main

import (
	"codexpet/reconstruction/companion"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	input := flag.String("input", "../evaluation/classification_holdout.json", "frozen test cases")
	output := flag.String("output", "../evaluation/classification_live.json", "metrics file")
	live := flag.Bool("live", false, "explicitly perform paid provider classification requests using stored key")
	flag.Parse()
	if !*live {
		fmt.Println("Use -live to explicitly run a provider benchmark. No requests sent.")
		return
	}
	b, err := os.ReadFile(*input)
	if err != nil {
		fmt.Println("test set unreadable")
		os.Exit(1)
	}
	var cases []companion.EmotionEvaluationCase
	if json.Unmarshal(b, &cases) != nil || len(cases) < 1 || len(cases) > 200 {
		fmt.Println("invalid test set")
		os.Exit(1)
	}
	s, err := companion.Start(nil)
	if err != nil {
		fmt.Println("local server unavailable")
		os.Exit(1)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	report, err := s.EvaluateEmotions(ctx, cases, os.Stdout)
	if err != nil {
		if errors.Is(err, companion.ErrNotConfigured) {
			fmt.Println("stored_key_unavailable")
		}
		fmt.Println("benchmark could not complete; no accuracy claim")
		os.Exit(1)
	}
	b, _ = json.MarshalIndent(report, "", "  ")
	if os.WriteFile(*output, b, 0600) != nil {
		fmt.Println("metrics could not be saved")
		os.Exit(1)
	}
	fmt.Printf("classification accuracy %.2f%% (%d/%d), fallbacks %d; reference labels await human review\n", report.Accuracy*100, report.Correct, report.Total, report.Fallbacks)
}
