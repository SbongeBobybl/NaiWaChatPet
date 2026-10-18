package emotion

// Entry describes one expression the pet can show. The classifier prompt, the
// protocol validation, the reply guidance, the action menu and the importer
// all read this table, so an expression is added in exactly one place.
type Entry struct {
	// ID is the protocol token and the animation folder name; Label is what
	// the user sees.
	ID, Label string
	// Auto lets the model choose the expression for a reply. The rest are
	// played by the pet itself: on a timer, in the fly game, from the menu.
	Auto bool
	// Rule tells the model when the expression fits and Tone how the reply
	// should then sound.
	Rule, Tone string
	// Posed marks an expression imported as a held key pose. Gesture is the
	// rig loop that keeps that pose alive, and Source the GIF it came from
	// when that is not named after the ID.
	Posed           bool
	Gesture, Source string
	// Scene marks a filmed sequence with an entry and an exit of its own,
	// which the pet settles into and out of like a held pose.
	Scene    bool
	Priority int
}

// Catalog lists every expression, the six filmed ones first.
var Catalog = []Entry{
	{ID: "happy", Label: "开心", Auto: true, Priority: 25,
		Rule: "明确的好消息或无伤害的笑话",
		Tone: "适当表现开心或善意笑意，不在悲伤、讽刺或严肃问题中大笑。"},
	{ID: "surprised", Label: "惊讶", Auto: true, Priority: 25,
		Rule: "真正出乎意料的消息",
		Tone: "适度表达意外，接着认真回应。"},
	{ID: "confused", Label: "疑惑", Auto: true, Priority: 25,
		Rule: "内容含糊、矛盾，需要澄清",
		Tone: "温和说明不确定处，需要时只问一个必要的澄清问题。"},
	{ID: "shy", Label: "害羞", Auto: true, Priority: 25,
		Rule: "被直接夸可爱",
		Tone: "面对善意夸奖略带腼腆，简短自然回应。"},
	{ID: "sad", Label: "伤心", Auto: true, Priority: 25,
		Rule: "用户失落或遇到困难，温柔共情",
		Tone: "用轻柔体贴的语气，不夸大自己的悲伤。"},
	{ID: "angry", Label: "生气", Auto: true, Priority: 25,
		Rule: "明确的欺负、恶意诬蔑、侮辱奶蛙或严重不公平",
		Tone: "对不公平表达温和的关心和有边界的不满，不对用户发火或辱骂。"},

	{ID: "love", Label: "喜爱", Auto: true, Posed: true, Gesture: "tilt", Priority: 22,
		Rule: "用户表达喜欢、想念或关心奶蛙",
		Tone: "语气亲近温暖，表达喜欢但不黏腻。"},
	{ID: "proud", Label: "得意", Auto: true, Posed: true, Gesture: "puff", Priority: 22,
		Rule: "奶蛙帮上了忙，或被夸能干",
		Tone: "带一点小得意，仍然谦和可爱。"},
	{ID: "cheer", Label: "欢呼", Auto: true, Posed: true, Gesture: "bounce", Priority: 28,
		Rule: "给用户加油打气，为用户的努力喝彩",
		Tone: "热情鼓励，给出具体的打气。"},
	{ID: "celebrate", Label: "庆祝", Auto: true, Posed: true, Gesture: "bounce", Priority: 28,
		Rule: "用户达成目标、通过考试，或生日、节日等值得庆祝的事",
		Tone: "真诚祝贺，分享喜悦。"},
	{ID: "greet", Label: "打招呼", Auto: true, Posed: true, Gesture: "rock", Priority: 22,
		Rule: "见面问候或道别",
		Tone: "亲切自然地问候或道别。"},
	{ID: "grateful", Label: "感谢", Auto: true, Posed: true, Gesture: "bow", Priority: 22,
		Rule: "用户帮了奶蛙、送礼物或表达善意，奶蛙想道谢",
		Tone: "真诚道谢，简短不客套。"},
	{ID: "agree", Label: "赞同", Auto: true, Posed: true, Gesture: "nod", Priority: 16,
		Rule: "明确认同用户的看法，或爽快答应请求",
		Tone: "干脆地表示认同或答应，再补充要点。"},
	{ID: "refuse", Label: "拒绝", Auto: true, Posed: true, Gesture: "shake", Priority: 28,
		Rule: "不能做或不该做的请求，需要温和而明确地拒绝",
		Tone: "温和而明确地拒绝，说明原因，尽量给出可行的替代。"},
	{ID: "ponder", Label: "思索", Auto: true, Posed: true, Gesture: "tilt", Priority: 16,
		Rule: "需要认真想一想的开放问题或两难选择",
		Tone: "认真思考后给出有条理的看法，不输出思考过程。"},
	{ID: "curious", Label: "好奇", Auto: true, Posed: true, Gesture: "tilt", Priority: 22,
		Rule: "用户提到新鲜有趣的事，奶蛙想听更多",
		Tone: "表现出兴趣，可以追问一个具体问题。"},
	{ID: "scared", Label: "害怕", Auto: true, Posed: true, Gesture: "tremble", Priority: 28,
		Rule: "恐怖、惊悚或吓人的内容",
		Tone: "有点害怕但不夸张，仍然把话说清楚。"},
	{ID: "worried", Label: "担心", Auto: true, Posed: true, Gesture: "breathe", Priority: 22,
		Rule: "用户身体不适、熬夜或冒险，奶蛙放心不下",
		Tone: "表达关心，提醒照顾好自己，不说教。"},
	{ID: "crying", Label: "哭泣", Auto: true, Posed: true, Gesture: "sob", Priority: 28,
		Rule: "非常伤心，或被深深感动到落泪，程度强于 sad",
		Tone: "情绪更浓的共情，轻柔克制，不渲染悲伤。"},
	{ID: "wronged", Label: "委屈", Auto: true, Posed: true, Gesture: "sigh", Priority: 22,
		Rule: "奶蛙被误解、被凶或被冤枉，程度轻于 angry",
		Tone: "带一点委屈地说明自己的感受，不指责用户。"},
	{ID: "disappointed", Label: "失望", Auto: true, Posed: true, Gesture: "sigh", Priority: 22,
		Rule: "期待落空、计划泡汤",
		Tone: "坦率表达遗憾，随后看向接下来能做的事。"},
	{ID: "embarrassed", Label: "尴尬", Auto: true, Posed: true, Gesture: "breathe", Priority: 22,
		Rule: "奶蛙说错、做错被指出，或场面尴尬",
		Tone: "不好意思地承认，简短更正。"},
	{ID: "sleepy", Label: "困倦", Auto: true, Posed: true, Gesture: "sigh", Priority: 16,
		Rule: "深夜、互道晚安或聊到犯困",
		Tone: "语气放慢放轻，可以提醒早点休息。"},
	{ID: "annoyed", Label: "不悦", Auto: true, Posed: true, Gesture: "huff", Priority: 22,
		Rule: "被反复捉弄或打扰，轻微不悦，程度轻于 angry",
		Tone: "轻微不满但有分寸，不对用户发火。"},
	{ID: "relieved", Label: "放松", Auto: true, Posed: true, Gesture: "sigh", Priority: 16,
		Rule: "虚惊一场或难题解决，松一口气",
		Tone: "轻松地表示放心。"},
	{ID: "hug", Label: "拥抱", Auto: true, Posed: true, Gesture: "rock", Priority: 22,
		Rule: "用户需要安慰或陪伴，给一个抱抱",
		Tone: "温柔安抚，先接住情绪，再陪伴。"},

	{ID: "smile", Label: "微笑", Posed: true, Gesture: "breathe", Source: "happy", Priority: 20},
	{ID: "excited", Label: "兴奋", Posed: true, Gesture: "bounce", Priority: 28},
	{ID: "focused", Label: "专注", Posed: true, Gesture: "breathe", Priority: 16},
	{ID: "bored", Label: "无聊", Posed: true, Gesture: "sigh", Priority: 16},
	{ID: "smug", Label: "傲娇", Posed: true, Gesture: "puff", Priority: 22},
	{ID: "dizzy", Label: "晕乎", Posed: true, Gesture: "sway", Priority: 22},
	{ID: "shocked", Label: "震住", Posed: true, Gesture: "tremble", Priority: 28},

	// The pet carries a box while it tidies the desktop. It outranks every
	// expression so a reply cannot cut the job short on screen.
	{ID: "organize", Label: "整理桌面", Scene: true, Priority: 45},
}

// Find returns the catalog entry for an expression.
func Find(id string) (Entry, bool) {
	for _, e := range Catalog {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// automatic lists neutral and every expression the model may choose.
func automatic() []string {
	out := []string{"neutral"}
	for _, e := range Catalog {
		if e.Auto {
			out = append(out, e.ID)
		}
	}
	return out
}

// Known reports whether a name is an expression the pet can be asked to play,
// including the older names kept for the local companionship lines.
func Known(name string) bool {
	if _, ok := Find(name); ok {
		return true
	}
	for _, legacy := range []string{"neutral", "laugh", "thinking", "comforting"} {
		if name == legacy {
			return true
		}
	}
	return false
}

// Canonical maps the older names that the local companionship lines still use
// onto the expressions that now show them.
func Canonical(name string) string {
	switch name {
	case "comforting":
		return "hug"
	case "thinking":
		return "ponder"
	}
	return name
}
