package emotion

import (
	"strings"
	"time"
)

func has(text string, terms ...string) bool {
	for _, v := range terms {
		if strings.Contains(text, v) {
			return true
		}
	}
	return false
}
func Analyze(e Cue) Decision {
	neutral := Decision{"neutral", 0, 0}
	if e.Stage != "error" && e.Stage != "cancelled" && e.Stage != "streaming" && strings.Contains(e.User, "我") && has(e.User, "不知道怎么办", "有些难过") {
		return Decision{"comforting", .9, 14 * time.Second}
	}
	u, r := strings.TrimSpace(e.User), strings.TrimSpace(e.Reply)
	if e.Stage == "completed" && len([]rune(u)) < 16 && has(u, "谢谢", "好多了", "好些了") && has(e.Context, "难过", "委屈", "压力", "焦虑") {
		return Decision{"comforting", .85, 12 * time.Second}
	}
	if e.Stage == "streaming" || e.Stage == "error" || e.Stage == "cancelled" {
		return neutral
	}
	technical := has(u, "证明", "推导", "代码", "算法", "函数", "SQL", "sql", "论文", "公式", "定理", "矩阵", "报错", "编译", "接口", "参数", "作业", "题目", "题为什么")
	if e.Stage == "waiting" {
		if technical || len([]rune(u)) > 80 {
			return Decision{"thinking", .92, 30 * time.Second}
		}
		return neutral
	}
	if e.Stage != "completed" {
		return neutral
	}
	quoted := has(u, "翻译", "引用", "这句话", "台词", "小说", "故事", "例句", "统计", "歌词")
	sarcasm := has(u, "呵呵", "真棒，又", "可真棒", "开心个鬼", "笑不出来", "有什么好笑", "高兴不起来", "哈哈，完了", "哈哈我又失败", "开心才怪", "反讽", "讽刺")
	if !quoted && (sarcasm || has(u, "我好难过", "我今天好难过", "我很难过", "我不开心", "我失眠", "我焦虑", "我撑不住", "我很委屈", "我心情很差", "我哭", "压力很大", "我失败", "我好烦", "我被", "我失去", "我害怕", "不知怎么办")) {
		return Decision{"comforting", .91, 14 * time.Second}
	}
	if technical || quoted {
		return neutral
	}
	if has(r, "慢慢来", "我陪你", "先缓一缓", "听起来很难受", "你不用急", "确实不容易", "理解你的难受", "我们一步一步", "先照顾好自己") {
		return Decision{"comforting", .87, 14 * time.Second}
	}
	if has(r, "不好意思", "脸红", "害羞", "被你夸") && has(u, "奶蛙", "你真", "你好", "可爱", "喜欢你", "夸你") {
		return Decision{"shy", .9, 12 * time.Second}
	}
	if has(r, "欺负人的做法不对", "这种做法不对", "太过分", "不该这样对待", "实在不公平", "替你生气") {
		return Decision{"angry", .89, 10 * time.Second}
	}
	if has(r, "我还没看明白", "还没弄清", "有点疑惑", "我也有些困惑", "需要你再说清", "这让我有点迷糊") {
		return Decision{"confused", .86, 12 * time.Second}
	}
	if has(r, "真没想到", "出乎意料", "居然", "惊到了", "太意外", "吓了一跳") {
		return Decision{"surprised", .87, 12 * time.Second}
	}
	if has(r, "有点遗憾", "听着有些伤心", "真让人难过", "有点失落", "我也有些难过") {
		return Decision{"sad", .82, 12 * time.Second}
	}
	if has(u, "哈哈哈", "笑死我", "太好玩", "笑得肚子疼") && !sarcasm {
		return Decision{"happy", .95, 12 * time.Second}
	}
	if has(r, "真开心", "太开心", "哈哈哈", "替你开心", "值得庆祝", "一起笑", "开心得想", "喜事") && has(u, "成功", "通过", "完成", "开心", "好消息", "录取", "获奖", "解决", "顺利", "终于", "笑") {
		return Decision{"happy", .91, 12 * time.Second}
	}
	return neutral
}
func (c *Controller) RequestEmotion(name string, confidence float64, context string) bool {
	if confidence < .78 {
		return false
	}
	return c.SetEmotion(name, confidence, 15*time.Second, context)
}
func (c *Controller) ConsumeLaugh(blocked bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if blocked || c.current == nil || c.laughConsumed {
		return false
	}
	if c.current.smooth {
		if c.current.intensity < .6 || c.current.confidence < MinimumConfidence {
			return false
		}
	} else if c.current.intensity < .85 {
		return false
	}
	if c.current.emotion != "happy" && c.current.emotion != "laugh" {
		return false
	}
	c.laughConsumed = true
	return true
}
