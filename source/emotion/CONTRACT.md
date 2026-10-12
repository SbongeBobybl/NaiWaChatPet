# Emotion track contract

Locomotion owns the body: idle, walk, run, jump, and attack, including displacement and facing. Emotion is a facial/upper overlay. The two tracks do not share one exclusive animator clip slot.

The legacy desktop player still draws full-character PNG plates from `animations/`. Those plates are the existing frame pipeline (same canvas, fps family, and idle in/out transitions). The scheduler below does not apply root motion on top of them. New plates are short windows of the reviewed clips, played between that clip family's existing `*_in` and `*_out` transitions.

## Entry points

Wire locomotion against this package (`source/emotion`):

| Call | File | Role |
| --- | --- | --- |
| `Start` | `source/emotion/scheduler.go` | Subscribe after locomotion is grounded |
| `RequestEmotion` / `RequestEmotionFrom` | `source/emotion/scheduler.go` | Ask for an overlay; may queue or reject |
| `GetEmotionState` | `source/emotion/scheduler.go` | `{emotion_id, emotion_phase, emotion_priority, emotion_playing, emotion_blocks_hover_punch}` |
| `OnEmotionChange` | `source/emotion/scheduler.go` | Push form of the same snapshot |
| `InterruptsLocomotionIdle` | `source/emotion/scheduler.go` | High-priority overlay may replace walk/run idle variants |
| `LocomotionSource` | `source/emotion/locomotion.go` | Emotion reads `GetLocomotionState` / `OnLocomotionChange` |
| `ForcedChoices` | `source/emotion/catalog.go` | Closed set the LLM emotion chooser must use |
| `emotions.json` | `source/emotion/emotions.json` | Ids, Chinese labels, priorities, clip paths |

`GetEmotionState` fields:

- `emotion_id`
- `emotion_phase`: `idle` | `in` | `loop` | `out`
- `emotion_priority`: `low` | `normal` | `high`
- `emotion_playing`
- `emotion_blocks_hover_punch` (true during in/loop/out)

`GetLocomotionState` fields this track reads:

- `action_name`
- `grounded`
- `facing`
- `busy_locomotion`
- `can_play_emotion`

`RequestEmotion(emotion_id, priority)` is the automatic path used by the LLM. `RequestEmotionFrom(..., CauseManual)` and `CausePlayer` are manual forced plays. Both beat automatic emotion. Neither starts `emotion_in` while jump or attack is busy.

## Conflict rules

- Jump or attack (`busy_locomotion`, or `action_name` jump/attack): queue the request. If an overlay is already playing, clear it and play `emotion_in` only after busy clears.
- Walk and run: overlay is allowed. Locomotion keeps displacement.
- High priority may interrupt a lower automatic overlay, and `InterruptsLocomotionIdle` tells locomotion it may drop walk/run idle variants. High priority never interrupts jump/attack.
- Player attack/jump and manual forced plays beat automatic emotion.
- Equal or lower automatic requests do not restart the clip that is already playing.

## Startup

1. Load animation resources (`animations/*/manifest.json` plus the legacy embedded body clips).
2. Locomotion reports ready: grounded, `can_play_emotion`, not busy.
3. Call `Scheduler.Start`. Until that snapshot arrives, `RequestEmotion` returns `not_ready`.
4. The LLM / companion may then call `RequestEmotion`.

## LLM set and playback

The forced choice list is `ForcedChoices()`: `neutral` plus every id in `emotions.json`. The running `NaiWaChatPet.exe` uses the same ids. Its analyzer prompt and `emotion.Analysis.Valid` allow-list were extended in place; `thinking` stays tolerated by the validator and is still forbidden by the prompt.

Each playable id maps to `animations/<id>/manifest.json`. `happy` also keeps the embedded full laugh when intensity is at least 0.6 (`toLaugh` / `video` / `fromLaugh`). `neutral` plays no clip.

The legacy loader rejects the whole library if frame files exceed 128MiB. Keep new plates inside that budget.

## Emotion table

Canonical data is `source/emotion/emotions.json`. Clip paths are relative to the repository root.

<!-- EMOTION_TABLE_START -->
| id | 中文 | priority | clip | transition in | transition out |
| --- | --- | --- | --- | --- | --- |
| neutral | 待机 | low | （无片段，回到待机） |  |  |
| happy | 开心 | normal | `animations/happy` | `surprised_in` | `surprised_out` |
| surprised | 惊讶 | high | `animations/surprised` | `surprised_in` | `surprised_out` |
| confused | 疑惑 | normal | `animations/confused` | `confused_in` | `confused_out` |
| shy | 害羞 | normal | `animations/shy` | `shy_in` | `shy_out` |
| sad | 伤心 | normal | `animations/sad` | `sad_in` | `sad_out` |
| angry | 生气 | high | `animations/angry` | `angry_in` | `angry_out` |
| proud | 得意 | normal | `animations/proud` | `angry_in` | `angry_out` |
| sleepy | 困倦 | low | `animations/sleepy` | `sad_in` | `sad_out` |
| love | 喜爱 | normal | `animations/love` | `shy_in` | `shy_out` |
| scared | 害怕 | high | `animations/scared` | `surprised_in` | `surprised_out` |
| excited | 兴奋 | high | `animations/excited` | `surprised_in` | `surprised_out` |
| bored | 无聊 | low | `animations/bored` | `confused_in` | `confused_out` |
| embarrassed | 尴尬 | normal | `animations/embarrassed` | `shy_in` | `shy_out` |
| curious | 好奇 | normal | `animations/curious` | `confused_in` | `confused_out` |
| relieved | 放松 | low | `animations/relieved` | `sad_in` | `sad_out` |
| disappointed | 失望 | normal | `animations/disappointed` | `sad_in` | `sad_out` |
| grateful | 感谢 | normal | `animations/grateful` | `shy_in` | `shy_out` |
| worried | 担心 | normal | `animations/worried` | `sad_in` | `sad_out` |
| smug | 傲娇 | normal | `animations/smug` | `angry_in` | `angry_out` |
| crying | 哭泣 | high | `animations/crying` | `sad_in` | `sad_out` |
| wronged | 委屈 | normal | `animations/wronged` | `shy_in` | `shy_out` |
| cheer | 欢呼 | high | `animations/cheer` | `surprised_in` | `surprised_out` |
| dizzy | 晕乎 | normal | `animations/dizzy` | `confused_in` | `confused_out` |
| focused | 专注 | low | `animations/focused` | `confused_in` | `confused_out` |
| greet | 打招呼 | normal | `animations/greet` | `shy_in` | `shy_out` |
| agree | 赞同 | low | `animations/agree` | `confused_in` | `confused_out` |
| refuse | 拒绝 | high | `animations/refuse` | `angry_in` | `angry_out` |
| ponder | 思索 | low | `animations/ponder` | `confused_in` | `confused_out` |
| annoyed | 不悦 | normal | `animations/annoyed` | `angry_in` | `angry_out` |
| celebrate | 庆祝 | high | `animations/celebrate` | `surprised_in` | `surprised_out` |
| hug | 拥抱 | normal | `animations/hug` | `shy_in` | `shy_out` |
| shocked | 震住 | high | `animations/shocked` | `surprised_in` | `surprised_out` |
<!-- EMOTION_TABLE_END -->
