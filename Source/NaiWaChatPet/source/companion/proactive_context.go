package companion

func (j *chatJob) emotionContext() []Message {
	history := append([]Message(nil), j.history...)
	if len(j.recentProactive) > 0 {
		history = append(history, Message{Role: "assistant", Content: j.recentProactive[len(j.recentProactive)-1]})
	}
	return history
}
