## Note for external contributors / 外部贡献者请注意

DeskBox is not merging external pull requests at this stage — please see [CONTRIBUTING.md](../CONTRIBUTING.md). Bug reports, ideas, and discussions are very welcome via Issues / Discussions.

DeskBox 现阶段暂不合并外部 Pull Request，请阅读 [CONTRIBUTING.md](../CONTRIBUTING.md)。欢迎通过 Issue / Discussion 参与反馈和讨论。

---

## Maintainer checklist / 维护者自查

- [ ] `dotnet test .\tests\DeskBox.Tests\DeskBox.Tests.csproj --no-restore --verbosity:minimal -p:Platform=x64` all green
- [ ] Serialization/boundary changes → ratchet updated on both layers (baseline count + AotStage literals)
- [ ] `DESKBOX_NATIVE_AOT` files touched → AOT retail publish verified, `packages.lock.json` restored after
- [ ] Commit messages carry only `Simon <1047078635@qq.com>` — no Co-Authored-By / generated-with trailers (hook + CI enforce)
- [ ] Docs/version numbers updated if user-visible (changelog, READMEs, freeze counts for UI contract tests)
