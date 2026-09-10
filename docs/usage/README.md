# jardec 使用指南

`jardec` 是一个面向 JAR 的命令行工具：它先用 JADX 反编译整个
JAR，再只对有明确失败证据的顶层类使用 Procyon 重试。成功的
Procyon 结果会替换最终输出中的对应 Java 文件，报告会保留每个类
最终使用的工具、重试原因和失败诊断。

本文中的 `jardec` 可替换为已构建的 `bin/jardec`，也可在源码仓库中
替换为 `go run ./cmd/jardec`。

## 1. 前置条件

需要准备：

- Go 1.24+（仅从源码运行或构建时需要）；
- 可执行的 `jadx`；
- Java 运行时；
- Procyon decompiler 的 JAR 文件；
- `javac`（仅 `patch-sources` 命令需要）。

构建二进制：

```bash
make build
./bin/jardec --help
```

每次 `decompile` 启动时，工具都会先运行
`java -jar <procyon-path> --help`。因此 Procyon 路径、JAR 完整性和
Java 运行时的问题会在 JADX 执行前暴露出来。

## 2. 反编译一个 JAR

最小示例：

```bash
jardec decompile \
  --input /path/to/app.jar \
  --output /path/to/decompiled-app \
  --jadx-path /path/to/jadx \
  --procyon-path /path/to/procyon-decompiler.jar
```

成功后输出目录结构如下：

```text
decompiled-app/
├── sources/       # 最终 Java 源码
├── resources/     # JADX 提取的资源
├── report.json    # 机器可读的逐类结果
└── report.txt     # 便于终端查看的摘要和逐类结果
```

`sources/` 中默认保留 JADX 结果。仅在某个顶层类被标记为失败候选、
且 Procyon 成功生成可用源码时，才会用 Procyon 的结果覆盖该类。
这不是对所有类重复反编译。

### 2.1 何时触发 Procyon 回退

回退只由可审计的失败信号触发，包括：

- 预期的 `.java` 文件没有生成；
- 输出为空或近似为空；
- JADX 针对该类报告错误；
- 输出中存在已知失败占位文本；
- 源码中带有 `JADX WARN` 标记。

首版完整性目标是顶层类；匿名类、合成类或 lambda 生成类不在完整性
保证范围内。

### 2.2 常用参数

| 参数 | 说明 |
| --- | --- |
| `--input`, `-i` | 输入 JAR 路径。 |
| `--output`, `-o` | 最终输出目录。 |
| `--jadx-path` | JADX 可执行文件路径；未指定时查找 `jadx`。 |
| `--procyon-path` | Procyon decompiler JAR 路径；通过 `java -jar` 调用。 |
| `--classpath` | 追加依赖 JAR 或包含 JAR 的目录，可重复指定；用于 Procyon 回退。 |
| `--temp-dir` | 临时工作目录根路径；不存在时自动创建。 |
| `--keep-temp` | 保留每个 Procyon 回退的隔离工作区，便于排错。 |
| `--retry-concurrency` | 同时运行的 Procyon 回退数；默认 CPU 核数。 |
| `--procyon-timeout` | 单个 Procyon 回退的最长时间；默认 `90s`。 |
| `--config` | 显式指定 `config.yaml`。 |

时长支持 Go duration 格式，例如 `30s`、`2m`。`--procyon-timeout` 和
`--retry-concurrency` 必须为正数。

### 2.3 添加依赖 classpath

依赖类型无法解析时，为 Procyon 提供 classpath：

```bash
jardec decompile \
  --input app.jar \
  --output out \
  --jadx-path /tools/jadx \
  --procyon-path /tools/procyon.jar \
  --classpath libs \
  --classpath third-party/legacy-api.jar
```

单个条目必须是可读的常规 `.jar` 文件。目录会以非递归方式展开其中的
`*.jar` 文件，并按文件名排序；空目录、缺失路径、非 JAR 文件都会在
工具启动前报错。重复路径会去重。

### 2.4 控制性能和排障信息

大量失败候选会启动多个 JVM。建议先从较小并发开始：

```bash
jardec decompile \
  --input app.jar --output out \
  --jadx-path /tools/jadx --procyon-path /tools/procyon.jar \
  --retry-concurrency 2 \
  --procyon-timeout 2m \
  --keep-temp
```

如果某个 Procyon 调用超时，该类会标为 `procyon_timeout`，其他候选类
仍会继续。Linux 上超时会终止该调用所属的整个进程组，以避免派生进程
持有输出管道而导致命令无法收尾。

JADX 每次运行都使用独立临时工作区以及独立的 `XDG_CONFIG_HOME` 和
`XDG_CACHE_HOME`，因此不依赖用户主目录可写。默认会清理临时工作区；
传入 `--keep-temp` 时，报告会给出失败回退的保留路径。

## 3. 配置文件

工具默认从当前工作目录开始，逐级向父目录查找 `config.yaml`。也可以
通过全局参数显式指定：

```bash
jardec --config /path/to/config.yaml decompile --input app.jar --output out
```

示例：

```yaml
jadx_path: /tools/jadx
procyon_path: /tools/procyon-decompiler.jar
javac_path: /usr/bin/javac
decompile_classpath:
  - libs
  - third-party/legacy-api.jar
default_retry_concurrency: 4
```

字段含义：

| 字段 | 用于 | 说明 |
| --- | --- | --- |
| `jadx_path` | `decompile` | 默认 JADX 路径。 |
| `procyon_path` | `decompile` | 默认 Procyon 路径。 |
| `javac_path` | `patch-sources` | 默认 `javac` 路径。 |
| `decompile_classpath` | `decompile`、`patch-sources` | 依赖条目；相对路径相对于配置文件所在目录。 |
| `default_retry_concurrency` | `decompile` | 未指定 CLI 参数时的默认回退并发数。 |

优先级是：命令行参数 > `config.yaml` > 内建默认值。`--classpath` 会在
配置文件的 `decompile_classpath` 后追加，并进行去重。

## 4. 阅读反编译报告

`report.txt` 适合快速查看总览；自动化处理应读取 `report.json`。其中
重要的顶层字段如下：

| 字段 | 含义 |
| --- | --- |
| `totalTopLevelClasses` | 识别到的顶层类总数。 |
| `jadxSucceeded` | 最终由 JADX 成功提供源码的类数。 |
| `retryCandidates` | 被判定需要 Procyon 回退的类数。 |
| `procyonRecovered` | 成功由 Procyon 恢复的类数。 |
| `finalFailed` | 回退后仍失败的类数。 |
| `totalElapsedMillis` | 整个流程的墙钟耗时。 |
| `retryElapsedMillis` | 回退阶段的墙钟耗时。 |
| `classes` | 每个顶层类的结果。 |

每个 `classes` 元素至少包含 `binaryName` 和 `status`，成功类还会通过
`origin` 标记为 `jadx` 或 `procyon`。回退类可能包含：

- `retryReasons`：触发回退的证据，例如 `jadx_warn`；
- `retryOutcome`：回退结论，例如 `procyon_timeout` 或
  `procyon_execution_failed`；
- `failureReason`：最终失败原因；
- `dependencyWarnings`：依赖解析相关提示；
- `procyonDiagnostics`：工具调用诊断。

`procyonDiagnostics` 包含退出码、受限 stdout/stderr、实际命令描述和
`elapsedMillis`。超时还会包含 `timedOut: true` 与 `timeoutMillis`。
`workspaceDisposition` 为 `cleaned` 表示临时目录已清理；为 `retained`
时可使用 `workspacePath` 检查保留的工作区。

## 5. 将修改后的 class 写回 JAR

`patch-classes` 用编译好的 `.class` 文件替换原 JAR 中同名的顶层类组：
顶层类及其 `$` 内部类作为一个整体处理。旧的、此次未生成的内部类会被
删除，其他资源文件会保留。

```bash
jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes \
  --output-jar app.patched.jar
```

先预览而不生成输出 JAR：

```bash
jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes \
  --output-jar app.patched.jar \
  --class com.example.Foo \
  --dry-run
```

`--class` 可重复指定，值必须是顶层二进制类名（例如
`com.example.Foo`，不能包含 `$`）。命令会写出：

```text
app.patched.jar.report.json
app.patched.jar.report.txt
```

只要归档实际发生了变化，原 JAR 中失效的签名文件会被移除，并在报告中
记录；没有变化时签名会保留。

## 6. 编译 Java 源码并写回 JAR

`patch-sources` 适合先从反编译输出中编辑少数 `.java` 文件，再编译并
回写原 JAR。源码目录须按包路径放置：

```text
edited-src/
└── com/example/Foo.java
```

示例：

```bash
jardec patch-sources \
  --input-jar app.jar \
  --sources-dir edited-src \
  --output-jar app.patched.jar \
  --class com.example.Foo \
  --javac-path /usr/bin/javac \
  --classpath libs/dependency.jar
```

`--class` 至少要提供一个，并且只能是顶层二进制类名。输入 JAR 会自动
加入编译 classpath，`--classpath` 用于追加其他依赖。`javac` 路径未
指定时使用配置文件的 `javac_path`，再回退到 PATH 中的 `javac`。
编译结果及诊断会附加到同名 patch 报告中。

## 7. 常见问题

### 找不到工具或 Procyon 预检失败

确认 `--jadx-path` 指向可执行文件，`--procyon-path` 指向存在的 JAR，
并确认 `java -jar /path/to/procyon.jar --help` 可运行。若依赖 PATH，
确保 `jadx`、`java`（以及 `patch-sources` 的 `javac`）可被当前 shell
找到。

### classpath 校验失败

单文件必须是可读的 `.jar`；目录必须直接包含至少一个 `.jar`。目录不
递归扫描，嵌套依赖目录应单独通过 `--classpath` 指定。

### Procyon 超时或回退失败

查看 `report.json` 中该类的 `procyonDiagnostics`。必要时使用
`--keep-temp` 重新运行以保留工作区；可降低 `--retry-concurrency` 减少
资源争抢，或适度提高 `--procyon-timeout`。

### 输出仍有不完整的类

检查该类的 `status`、`origin` 和 `retryReasons`。如果状态为 `failed`，
报告中的 `failureReason` 与诊断可用于判断是工具失败、超时还是依赖缺失。
对不在顶层类基线内的匿名/合成/lambda 类，不应将未完整恢复视为 v1 的
保证范围。

## 8. 开发环境中的真实 JAR 验证

真实工具集成测试默认跳过，不会下载工具或搜索系统路径。需要时明确提供
三个路径：

```bash
JARDEC_INTEGRATION_JADX_PATH=/path/to/jadx \
JARDEC_INTEGRATION_PROCYON_PATH=/path/to/procyon.jar \
JARDEC_INTEGRATION_JAR=/path/to/input.jar \
go test ./internal/pipeline -run TestRealToolDecompileIntegration -count=1 -v
```

常规单元测试可运行：

```bash
make test
```
