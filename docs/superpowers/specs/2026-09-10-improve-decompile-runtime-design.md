---
cosp_change: improve-decompile-runtime
role: technical-design
canonical_spec: openspec
---

# 反编译运行时可靠性与性能设计

## 决策

### 工作区先于工具创建

`Engine.Run` 在调用 Procyon 预检前创建 `TempDir`（非空时），再让 jadx 与每个 Procyon 重试从该根目录建立各自的子工作区。创建失败直接返回带路径的错误，避免先创建用户输出目录后才暴露配置问题。

### 每次 jadx 调用使用工作区内 XDG 目录

`ExecuteJadx` 创建 `xdg-config` 与 `xdg-cache`，并在 jadx 的 `CommandSpec.Env` 中设置 `XDG_CONFIG_HOME` 和 `XDG_CACHE_HOME`。这样不依赖可写的用户目录，也不会让并发或连续运行共享插件状态。`RunJadx` 将构造命令抽成可描述的 spec；失败包装复用 Procyon 的受限输出规则，但以 jadx 作为来源。

### 早期 classpath 校验

CLI 展开 classpath 前，对单个文件条目进行 `Stat`、常规文件、可读性和 `.jar` 扩展名校验。目录保留现有非递归、按名称排序的 JAR 展开。不存在条目不再被静默带入 Procyon 命令。

### 回退耗时是附加诊断

重试 worker 围绕 Procyon 进程计时，并在 `ProcyonDiagnostics` 中写入毫秒数。报告保留已有状态、来源、失败原因和总回退墙钟耗时；文本报告只标记诊断可用，避免逐类输出膨胀。

### 单类 Procyon 有界执行

新增 `--procyon-timeout`，默认 90 秒。每个 worker 为一次 Procyon 调用建立独立 `context.WithTimeout`；超时时终止该 JVM，结果分类为 `procyon_timeout`，但其他类继续执行。诊断记录 `timedOut: true`、限制值与实际耗时；零或负时长在 CLI 校验阶段拒绝。

为避免 Procyon 派生进程持有 stdout/stderr 管道导致父 `Wait` 阻塞，Unix 实现将每次工具调用放入独立进程组；context 取消时先向整个组发送终止信号，再由有界 `WaitDelay` 强制收尾。非 Unix 平台保留标准库取消路径，保证编译与确定性降级。

### 真实工具测试显式启用

集成测试读取 `JARDEC_INTEGRATION_JADX_PATH`、`JARDEC_INTEGRATION_PROCYON_PATH` 与 `JARDEC_INTEGRATION_JAR`。任一缺失即 `Skip`；存在时在测试临时目录运行完整 CLI/引擎流程，并验证报告与输出布局。测试不下载工具、不访问未提供路径。

## 数据流

```text
CLI 配置
  -> 准备 TempDir + 验证 classpath
  -> Procyon 预检
  -> jadx 工作区 + XDG 环境
  -> 分类
  -> 逐类 Procyon（计时）
  -> 合并 + 报告
```

## 测试策略

- 单元测试覆盖临时根目录、无效 classpath、jadx 命令环境、jadx 失败错误和重试耗时。
- 管线测试覆盖失败时不创建不应存在的输出、工作区清理及报告兼容性。
- 集成测试默认跳过；由显式环境变量在具备真实工具的环境中启用。

## 风险

- XDG 隔离不加载用户全局插件；这是为可重复运行作出的有意取舍。
- 严格 classpath 校验是行为收紧，需在 README 中明确说明。
- 逐类 JVM 启动仍是主要成本；本变更提供数据来选择 `--retry-concurrency`，不破坏逐类隔离约束。
