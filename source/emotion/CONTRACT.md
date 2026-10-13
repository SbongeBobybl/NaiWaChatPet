# Emotion track contract

Locomotion owns the body: idle, walk, run, jump, and attack, including displacement and facing. Emotion is a facial/upper overlay. The two tracks do not share one exclusive animator clip slot.

The legacy desktop player still draws full-character PNG plates from `animations/`. Legacy emotions keep their original plates and transitions (`angry`, `confused`, `sad`, `shy`, `surprised`, plus the embedded happy laugh). Every other emotion is a keyframe performance of the reference character, built by `tools/render_emotions.py`. Peak poses are image-to-image edits of `source/emotion/reference-neutral.jpg`, locked to the same side-view frog. Frame 0 of each new clip (`animations/<id>`, `<id>_in`, `<id>_out`) is `source/emotion/neutral-reference.png` byte for byte. In-betweens are drawn partial poses when an arm, a crouch, or a bow moves, played in order so the action eases onto the peak. Small face changes are an optical-flow morph sampled across the blink. A morph is not used when it would ghost a limb or a second eye. They are not crops of the legacy plates, not an affine squash with painted eyes, and not a different shaded character. Standing poses that share a silhouette take a small sideways weight shift so each one reads apart from the others. Each new clip has its own `*_in` and `*_out`. Feet stay on the ground line. The scheduler does not add root motion.

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
| happy | 开心 | normal | `animations/happy` | `happy_in` | `happy_out` |
| surprised | 惊讶 | high | `animations/surprised` | `surprised_in` | `surprised_out` |
| confused | 疑惑 | normal | `animations/confused` | `confused_in` | `confused_out` |
| shy | 害羞 | normal | `animations/shy` | `shy_in` | `shy_out` |
| sad | 伤心 | normal | `animations/sad` | `sad_in` | `sad_out` |
| angry | 生气 | high | `animations/angry` | `angry_in` | `angry_out` |
| proud | 得意 | normal | `animations/proud` | `proud_in` | `proud_out` |
| sleepy | 困倦 | low | `animations/sleepy` | `sleepy_in` | `sleepy_out` |
| love | 喜爱 | normal | `animations/love` | `love_in` | `love_out` |
| scared | 害怕 | high | `animations/scared` | `scared_in` | `scared_out` |
| excited | 兴奋 | high | `animations/excited` | `excited_in` | `excited_out` |
| bored | 无聊 | low | `animations/bored` | `bored_in` | `bored_out` |
| embarrassed | 尴尬 | normal | `animations/embarrassed` | `embarrassed_in` | `embarrassed_out` |
| curious | 好奇 | normal | `animations/curious` | `curious_in` | `curious_out` |
| relieved | 放松 | low | `animations/relieved` | `relieved_in` | `relieved_out` |
| disappointed | 失望 | normal | `animations/disappointed` | `disappointed_in` | `disappointed_out` |
| grateful | 感谢 | normal | `animations/grateful` | `grateful_in` | `grateful_out` |
| worried | 担心 | normal | `animations/worried` | `worried_in` | `worried_out` |
| smug | 傲娇 | normal | `animations/smug` | `smug_in` | `smug_out` |
| crying | 哭泣 | high | `animations/crying` | `crying_in` | `crying_out` |
| wronged | 委屈 | normal | `animations/wronged` | `wronged_in` | `wronged_out` |
| cheer | 欢呼 | high | `animations/cheer` | `cheer_in` | `cheer_out` |
| dizzy | 晕乎 | normal | `animations/dizzy` | `dizzy_in` | `dizzy_out` |
| focused | 专注 | low | `animations/focused` | `focused_in` | `focused_out` |
| greet | 打招呼 | normal | `animations/greet` | `greet_in` | `greet_out` |
| agree | 赞同 | low | `animations/agree` | `agree_in` | `agree_out` |
| refuse | 拒绝 | high | `animations/refuse` | `refuse_in` | `refuse_out` |
| ponder | 思索 | low | `animations/ponder` | `ponder_in` | `ponder_out` |
| annoyed | 不悦 | normal | `animations/annoyed` | `annoyed_in` | `annoyed_out` |
| celebrate | 庆祝 | high | `animations/celebrate` | `celebrate_in` | `celebrate_out` |
| hug | 拥抱 | normal | `animations/hug` | `hug_in` | `hug_out` |
| shocked | 震住 | high | `animations/shocked` | `shocked_in` | `shocked_out` |
<!-- EMOTION_TABLE_END -->
