---
change: improve-decompile-runtime
design-doc: docs/superpowers/specs/2026-09-10-improve-decompile-runtime-design.md
base-ref: 465e66cc462900fc05b989a267367896a55b9253
archived-with: 2026-09-10-improve-decompile-runtime
---

# 反编译运行时可靠性与性能实施计划

> **给执行 agent：** 必须使用 `subagent-driven-development`（推荐）或 `executing-plans` 逐任务实施；每个步骤使用复选框追踪。

**目标：** 让指定临时目录、JADX 运行环境、classpath 输入和 Procyon 回退耗时都具备可验证、可诊断的行为，并提供显式启用的真实工具回归测试。

**架构：** `Engine.Run` 先验证可创建的临时根目录，再运行 Procyon 预检；`ExecuteJadx` 在独立 workspace 内创建 XDG 配置/缓存目录，并把它们作为命令环境传递。CLI 在目录展开之前严格验证单个 classpath 文件；重试 worker 只增加耗时采集，继续使用现有隔离工作区和报告模型。

**技术栈：** Go 标准库、`urfave/cli/v2`、现有 fake `decompiler.Runner`、`go test`。

**规格：** `docs/superpowers/specs/2026-09-10-improve-decompile-runtime-design.md`；`openspec/changes/improve-decompile-runtime/specs/`

## 全局约束

- 保持 JADX 优先、只对具备显式失败证据的顶层类逐类回退 Procyon 的既有流程。
- 不下载、不打包 JADX 或 Procyon；真实工具测试只读取明确设置的环境变量路径。
- 每个 Procyon 回退保留隔离临时工作区；并发 worker 不直接写最终输出目录。
- 保持已有报告字段、状态、来源和失败原因兼容；新增字段仅为附加诊断。
- 遵循 TDD：先增加失败测试，再实施最小代码，最后运行对应测试并提交。

---

## 文件结构

- `internal/pipeline/engine.go`：在任何工具调用前创建可选临时根目录，并包装 JADX 启动失败诊断。
- `internal/pipeline/jadx.go`：建立每次 JADX workspace 的 XDG 目录并传递给 decompiler 层。
- `internal/decompiler/runner.go`：让 `JadxConfig` 和 `RunJadx` 承载环境变量，并提供可描述的 JADX 命令。
- `internal/cli/config.go`：严格验证单文件 classpath 条目，目录展开规则不变。
- `internal/pipeline/retry.go`：采集单类 Procyon 进程耗时并携带到管线结果。
- `internal/report/report.go`：序列化单类 Procyon 耗时，文本报告保持简洁的诊断可用标记。
- `internal/pipeline/*_test.go`、`internal/decompiler/runner_test.go`、`internal/cli/config_test.go`：单元和管线回归测试。
- `internal/pipeline/integration_test.go`：由显式环境变量启用的真实工具端到端测试。
- `README.md`：记录新增约束、环境隔离、并发调优与集成测试用法。

## 任务 1：临时根目录与 JADX 环境隔离

**文件：**

- 修改：`internal/pipeline/engine.go`
- 修改：`internal/pipeline/jadx.go`
- 修改：`internal/decompiler/runner.go`
- 测试：`internal/pipeline/engine_test.go`
- 测试：`internal/pipeline/jadx_test.go`
- 测试：`internal/decompiler/runner_test.go`

**接口：**

- 消费：`Config.TempDir string`、`JadxWorkspaceConfig.BaseTempDir string`。
- 产出：`JadxConfig.Env []string`，其中仅追加 `XDG_CONFIG_HOME=<workspace>/xdg-config` 与 `XDG_CACHE_HOME=<workspace>/xdg-cache`；JADX 失败错误包含 exit code、命令与经 `TruncateDiagnostic` 限制后的 stdout/stderr。

- [ ] **步骤 1：先写失败测试**

  在 `engine_test.go` 增加两个测试：`TempDir` 为尚不存在的子目录时，fake Procyon 预检和 fake JADX 都能执行且根目录被创建；`TempDir` 指向普通文件时，`Engine.Run` 返回包含该路径的创建错误，并断言两个 runner 都未收到命令。保留预检失败时不创建输出目录的现有断言。

  在 `jadx_test.go` 的 fake runner 中断言 `spec.Env` 恰含 workspace 下的 `XDG_CONFIG_HOME` 与 `XDG_CACHE_HOME`，并用 `os.Stat` 确认两个目录存在。

  在 `runner_test.go` 增加 `TestRunJadxPassesConfiguredEnvironment`：传入 `JadxConfig{Env: []string{"XDG_CONFIG_HOME=/tmp/config", "XDG_CACHE_HOME=/tmp/cache"}}`，断言 fake runner 收到相同的 `CommandSpec.Env`。

- [ ] **步骤 2：运行测试确认失败**

  运行：`go test ./internal/pipeline ./internal/decompiler -run 'Test(EngineCreatesMissingTempRoot|EngineRejectsUncreatableTempRoot|ExecuteJadx.*XDG|RunJadxPassesConfiguredEnvironment)' -count=1`

  预期：失败，原因是引擎尚未建立 `TempDir`，且 JADX 命令尚未携带 XDG 环境。

- [ ] **步骤 3：实施最小代码**

  在 `Engine.Run` 的任何 runner 调用之前，当 `cfg.TempDir != ""` 时执行 `os.MkdirAll(cfg.TempDir, 0o755)`；失败包装为 `fmt.Errorf("create temp directory %q: %w", cfg.TempDir, err)`。不要创建最终输出目录后才报错。

  在 `ExecuteJadx` 的 `rootDir` 下创建 `xdg-config` 和 `xdg-cache`，失败时返回 workspace 错误；将两个赋值放入 `JadxConfig.Env`。给 `JadxConfig` 增加 `Env []string`，`RunJadx` 将其原样写入 `CommandSpec.Env`。增加 `JadxCommand`/`DescribeCommand` 的等价构造并在 `Engine.Run` 的 JADX 错误路径中返回工具名、exit code、命令、有限 stdout/stderr；不要改变成功路径或报告分类。

- [ ] **步骤 4：运行针对性测试确认通过**

  运行：`go test ./internal/pipeline ./internal/decompiler -run 'Test(EngineCreatesMissingTempRoot|EngineRejectsUncreatableTempRoot|ExecuteJadx.*XDG|RunJadxPassesConfiguredEnvironment|EngineStopsBeforeJadxWhenProcyonPreflightFails)' -count=1`

  预期：PASS；错误 TempDir 时 runner 未调用，成功运行的 JADX 命令具备两个 workspace 内 XDG 变量。

- [ ] **步骤 5：提交**

  将 `openspec/changes/improve-decompile-runtime/tasks.md` 的 1.1–1.2 标记为 `[x]`，随后：

  ```bash
  git add internal/pipeline/engine.go internal/pipeline/jadx.go internal/pipeline/engine_test.go internal/pipeline/jadx_test.go internal/decompiler/runner.go internal/decompiler/runner_test.go openspec/changes/improve-decompile-runtime/tasks.md
  git commit -m "feat: isolate jadx runtime workspace"
  ```

## 任务 2：严格 classpath 校验

**文件：**

- 修改：`internal/cli/config.go`
- 修改：`internal/cli/config_test.go`
- 测试：`internal/cli/source_patch_config_test.go`（若其复用 `expandClasspathEntry` 的断言需要更新）

**接口：**

- 消费：`expandClasspathEntry(entry string) ([]string, error)`。
- 产出：单文件条目只能是存在、可读、普通文件且文件名后缀为 `.jar`（大小写不敏感）；目录仍非递归读取、按名称排序，去重顺序仍由 `ValidateConfig` 保持。

- [ ] **步骤 1：先替换过时的失败测试**

  将 `TestValidateConfigAcceptsNonexistentClasspathFileEntry` 改为表驱动测试 `TestValidateConfigRejectsInvalidClasspathFileEntry`，覆盖：不存在的 `missing.jar`、目录外普通 `notes.txt`、以及权限设为 `0o000` 的 `unreadable.jar`（仅当当前平台能观测不可读时断言）。每项断言错误包含 `classpath` 和原始路径。

  再增加正向测试：创建实际 `valid.JAR`，经 `ValidateConfig` 后保留该路径；继续保留现有目录排序、目录无 JAR、配置路径重基和首次出现去重测试。

- [ ] **步骤 2：运行测试确认失败**

  运行：`go test ./internal/cli -run 'TestValidateConfig(RejectsInvalidClasspathFileEntry|AcceptsSingleJar|ExpandsConfigRelativeClasspathDirectory)' -count=1`

  预期：失败，因为缺失条目目前被原样透传，非 JAR 和不可读单文件尚未被拒绝。

- [ ] **步骤 3：实施最小代码**

  在 `expandClasspathEntry` 对非目录条目执行：`os.Stat` 错误直接返回 `fmt.Errorf("stat classpath entry %q: %w", entry, err)`；拒绝非 `Mode().IsRegular()` 的对象、拒绝 `!isJarPath(entry)`，再用 `os.Open` 与 `Close` 验证可读。对目录中的每个 `.jar` 子项同样只接受常规文件；返回的错误带输入路径。不得改变目录非递归、排序以及 `ValidateConfig` 的首次出现去重顺序。

- [ ] **步骤 4：运行针对性测试确认通过**

  运行：`go test ./internal/cli -count=1`

  预期：PASS；无效的单条 classpath 在启动工具前被拒绝，现有目录展开行为不变。

- [ ] **步骤 5：提交**

  将 `openspec/changes/improve-decompile-runtime/tasks.md` 的 1.3–1.4 标记为 `[x]`，随后：

  ```bash
  git add internal/cli/config.go internal/cli/config_test.go internal/cli/source_patch_config_test.go openspec/changes/improve-decompile-runtime/tasks.md
  git commit -m "feat: validate decompile classpath entries"
  ```

## 任务 3：逐类 Procyon 耗时诊断

**文件：**

- 修改：`internal/pipeline/retry.go`
- 修改：`internal/pipeline/engine.go`
- 修改：`internal/report/report.go`
- 测试：`internal/pipeline/retry_test.go`
- 测试：`internal/pipeline/engine_test.go`
- 测试：`internal/report/report_test.go`

**接口：**

- 消费：`RetryResult.Diagnostics decompiler.RunResult` 与现有 `ProcyonDiagnostics`。
- 产出：`RetryResult.ElapsedMillis int64`、`ProcyonDiagnostics.ElapsedMillis int64`（JSON key `elapsedMillis`）；所有已完成的回退记录该字段，值不小于零。

- [ ] **步骤 1：先写失败测试**

  在 `retry_test.go` 使用一个会返回的 fake runner 执行单类回退，断言结果的 `ElapsedMillis >= 0`；在 `engine_test.go` 的 Procyon 失败和恢复场景断言 `ProcyonDiagnostics.ElapsedMillis >= 0`，同时保持 `RetryOutcome`、`Origin` 与现有 stdout/stderr 断言不变；在 `report_test.go` 验证 JSON 出现 `"elapsedMillis"`，文本仍只出现 `procyonDiagnostics=available` 而不是每类耗时详情。

- [ ] **步骤 2：运行测试确认失败**

  运行：`go test ./internal/pipeline ./internal/report -run 'Test(ExecuteSingleRetry|Engine.*Procyon|WriteJSON.*Procyon|RenderText.*Procyon)' -count=1`

  预期：失败，原因是 `RetryResult` 与 `ProcyonDiagnostics` 尚无单类耗时字段。

- [ ] **步骤 3：实施最小代码**

  在 `executeSingleRetry` 调用 `RunProcyon` 前记录 `time.Now()`，调用后以 `time.Since(started).Milliseconds()` 填入 `RetryResult.ElapsedMillis`；即使命令返回错误也赋值。把该值映射到 `ProcyonDiagnostics.ElapsedMillis`，为报告结构增加 JSON 字段。保持 `RetryElapsedMillis` 作为总墙钟耗时，保持文本报告的紧凑格式。

- [ ] **步骤 4：运行针对性测试确认通过**

  运行：`go test ./internal/pipeline ./internal/report -count=1`

  预期：PASS；恢复和失败的每条完成回退都有非负耗时，既有结果分类未改变。

- [ ] **步骤 5：提交**

  将 `openspec/changes/improve-decompile-runtime/tasks.md` 的 3.1–3.2 标记为 `[x]`，随后：

  ```bash
  git add internal/pipeline/retry.go internal/pipeline/retry_test.go internal/pipeline/engine.go internal/pipeline/engine_test.go internal/report/report.go internal/report/report_test.go openspec/changes/improve-decompile-runtime/tasks.md
  git commit -m "feat: report per-class procyon timing"
  ```

## 任务 4：真实工具集成测试与文档

**文件：**

- 新建：`internal/pipeline/integration_test.go`
- 修改：`README.md`
- 修改：`openspec/changes/improve-decompile-runtime/tasks.md`

**接口：**

- 消费：`JARDEC_INTEGRATION_JADX_PATH`、`JARDEC_INTEGRATION_PROCYON_PATH`、`JARDEC_INTEGRATION_JAR`。
- 产出：未配置任一变量即 `t.Skip` 的端到端测试；配置完整时调用 `pipeline.Engine`，验证 `report.json` 可解析、顶层类总数大于零、成功与失败数之和等于总数，并且 `sources/` 存在。

- [ ] **步骤 1：先写默认跳过的集成测试**

  新建 `internal/pipeline/integration_test.go`，在测试开头用 `t.Setenv` 以外的 `os.Getenv` 读取三个变量；任一为空执行 `t.Skip("set JARDEC_INTEGRATION_JADX_PATH, JARDEC_INTEGRATION_PROCYON_PATH, and JARDEC_INTEGRATION_JAR to enable")`。完整配置时为输出与临时目录使用 `t.TempDir()`，不下载文件、不搜索外部目录。

- [ ] **步骤 2：运行默认测试确认跳过行为**

  运行：`go test ./internal/pipeline -run TestRealToolDecompileIntegration -count=1 -v`

  预期：PASS，并报告 `SKIP`，除非调用环境明确设置了全部三个变量。

- [ ] **步骤 3：实施完整断言与文档**

  用 `pipeline.Engine{}` 和 `pipeline.Config{InputPath: jar, OutputDir: out, JadxPath: jadx, ProcyonPath: procyon, TempDir: work, RetryConcurrency: 1}` 调用完整管线。解析生成的 `report.json`，断言统计恒等式和 `sources/` 目录。README 增加：`--temp-dir` 可自动创建、单文件 classpath 必须是可读 JAR、JADX 使用隔离 XDG 目录、`--retry-concurrency` 的调优依据，以及三变量的真实集成测试命令。

- [ ] **步骤 4：运行完整验证**

  运行：`gofmt -w internal/decompiler/runner.go internal/decompiler/runner_test.go internal/pipeline/engine.go internal/pipeline/engine_test.go internal/pipeline/jadx.go internal/pipeline/jadx_test.go internal/pipeline/retry.go internal/pipeline/retry_test.go internal/report/report.go internal/report/report_test.go internal/cli/config.go internal/cli/config_test.go internal/pipeline/integration_test.go && go test ./... -count=1`

  若三个变量均明确提供，再运行：`go test ./internal/pipeline -run TestRealToolDecompileIntegration -count=1 -v`。

  预期：完整单元套件 PASS；真实工具测试仅在显式配置时执行且报告、源码目录与统计恒等式成立。

- [ ] **步骤 5：勾选任务并提交**

  将 `openspec/changes/improve-decompile-runtime/tasks.md` 的 4.1–4.3 标记为 `[x]`，随后：

  ```bash
  git add internal/pipeline/integration_test.go README.md openspec/changes/improve-decompile-runtime/tasks.md
  git commit -m "test: add opt-in decompiler integration coverage"
  ```

## 计划自检

- 覆盖性：任务 1 覆盖 TempDir 创建、XDG 隔离与 JADX 启动诊断；任务 2 覆盖严格 classpath；任务 3 覆盖逐类耗时且不改变分类；任务 4 覆盖可选真实工具验证和文档。
- 无占位符：每一步指定了测试位置、命令、预期结果和最小实现方向。
- 类型一致性：任务 1 的 `JadxConfig.Env []string` 由 `ExecuteJadx` 生产并由 `RunJadx` 消费；任务 3 的 `RetryResult.ElapsedMillis int64` 映射为 `ProcyonDiagnostics.ElapsedMillis int64`。

## 任务 5：单类 Procyon 超时边界

**文件：**

- 修改：`internal/cli/app.go`、`internal/cli/config.go`
- 修改：`cmd/jardec/main.go`
- 修改：`internal/pipeline/engine.go`、`internal/pipeline/retry.go`
- 修改：`internal/report/report.go`
- 测试：`internal/cli/app_test.go`、`internal/pipeline/retry_test.go`、`internal/pipeline/engine_test.go`

**接口：**

- 消费：`--procyon-timeout duration`，默认 `90s`，值必须大于零。
- 产出：`Config.ProcyonTimeout time.Duration`、`ProcyonDiagnostics.TimedOut bool`、`TimeoutMillis int64` 和 retry outcome `procyon_timeout`。

- [ ] **步骤 1：写失败测试**

  在 CLI 测试验证默认 `90s`、显式 `250ms` 透传、`0s` 与负 duration 被拒绝。在 retry 测试使用会等待 `ctx.Done()` 的 runner，设置 `20ms` 后断言该类为 timeout；再添加第二个立即成功类，断言它仍完成。在 engine 测试断言 timeout 类为失败、`RetryOutcome == "procyon_timeout"`，且诊断包含 `TimedOut`、`TimeoutMillis == 20` 与非负耗时。

- [ ] **步骤 2：运行失败测试**

  运行：`go test ./internal/cli ./internal/pipeline -run 'Test(.*ProcyonTimeout|.*Timeout)' -count=1`

  预期：失败，因为 CLI、配置、worker context 和报告尚未提供 timeout 语义。

- [ ] **步骤 3：最小实现**

  使用 `urfave/cli.DurationFlag` 定义 `--procyon-timeout`，默认 `90*time.Second`；在 CLI 校验中拒绝非正值。将该值透传到 `pipeline.Config`、`ProcyonRetryConfig`；`executeSingleRetry` 用 `context.WithTimeout(ctx, cfg.Timeout)` 调用 Procyon，并仅在子 context 的 `DeadlineExceeded` 时标记 `TimedOut`。将该结果映射为 `procyon_timeout` 和附加报告字段，其他 worker 不取消。

- [ ] **步骤 4：验证并提交**

  运行：`gofmt -w cmd/jardec/main.go internal/cli/app.go internal/cli/config.go internal/cli/app_test.go internal/pipeline/engine.go internal/pipeline/retry.go internal/pipeline/retry_test.go internal/pipeline/engine_test.go internal/report/report.go && go test ./... -count=1`

  然后将 OpenSpec 5.1–5.3 勾为 `[x]`，更新 README 的 timeout 说明并提交：

  ```bash
  git add cmd/jardec/main.go internal/cli/app.go internal/cli/config.go internal/cli/app_test.go internal/pipeline/engine.go internal/pipeline/retry.go internal/pipeline/retry_test.go internal/pipeline/engine_test.go internal/report/report.go README.md
  git commit -m "feat: bound per-class procyon retries"
  ```
