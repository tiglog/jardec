# jardec

一个基于 Go 的命令行工具，用于对 JAR 执行 **jadx 优先、cfr 回退** 的反编译流程。

## 当前能力

- 用 `jadx` 对整个 JAR 做首轮反编译
- 以 **top-level class** 作为完整性基线
- 对可疑结果执行严格判定，包含 `JADX WARN`
- 对失败项逐类使用 `cfr` 补偿
- 允许 `cfr` 覆盖 retry 项的 `jadx` 输出
- 生成 `report.json` 和 `report.txt`
- 将已编译的 `.class` 变更按 top-level class group 打回原 JAR
- 支持 patch dry-run、显式 class group 选择和持久化 patch 报告
- 在反编译报告中输出 retry 候选数、retry 结果和耗时摘要

## 依赖

- Go 1.24+
- 本地可用的 `jadx`
- 本地可用的 `cfr`

## 安装 `jadx` 和 `cfr`

### 安装 `jadx`

推荐直接使用官方发布包，下载后解压，并把 `bin/jadx` 加到 `PATH`，或者把绝对路径写进 `config.yaml`。

```bash
# 例如：下载并解压后
export PATH="/path/to/jadx/bin:$PATH"

# 验证
jadx --version
```

如果你的系统包管理器提供了 `jadx`，也可以直接用包管理器安装，但建议优先确认版本是否符合预期。

### 安装 `cfr`

`cfr` 通常以 jar 形式发布。你可以直接把 `cfr.jar` 的绝对路径写进 `config.yaml`，也可以为它准备一个本地包装脚本后再把脚本路径配置给 `jardec`。

示例包装脚本：

```bash
#!/usr/bin/env bash
exec java -jar /path/to/cfr.jar "$@"
```

保存为例如 `/usr/local/bin/cfr` 后，赋予可执行权限：

```bash
chmod +x /usr/local/bin/cfr

# 验证
cfr --help
```

如果你不想放到 `PATH`，也可以直接把包装脚本或 `cfr.jar` 的绝对路径写进 `config.yaml`。

## 常用命令

```bash
make test
make build
make run
```

## 直接运行

```bash
go run ./cmd/jardec \
  --input sample.jar \
  --output out \
  --jadx-path /path/to/jadx \
  --cfr-path /path/to/cfr
```

常用参数：

- `--input`：输入 JAR 路径
- `--output`：输出目录
- `--jadx-path`：`jadx` 可执行文件路径
- `--cfr-path`：`cfr` 可执行文件或包装脚本路径
- `--temp-dir`：临时目录根路径
- `--keep-temp`：保留中间工作目录
- `--retry-concurrency`：`cfr` 回退并发数

## Patch 已编译 class 回原 JAR

`patch-classes` 用于把已经重新编译好的 `.class` 文件增量写回原始 JAR。v1 **只支持 compiled-class-only**：你需要先在外部完成 Java 编译，再把编译产物目录交给 `jardec`。

命令格式：

```bash
go run ./cmd/jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes/java/main \
  --output-jar app.patched.jar
```

预览 patch 计划而不落盘：

```bash
go run ./cmd/jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes/java/main \
  --output-jar app.patched.jar \
  --dry-run
```

只 patch 指定 top-level class group：

```bash
go run ./cmd/jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes/java/main \
  --output-jar app.patched.jar \
  --class com.example.Foo \
  --class com.example.Bar
```

行为约定：

- 自动扫描 `--classes-dir` 下的 `.class` 文件
- 可用 `--class` 精确限制只处理某些 top-level binary class name
- 可用 `--dry-run` 先预览会替换哪些 entries、会删除哪些陈旧内部类和签名文件
- 以 **top-level class group** 作为替换单元，例如 `Foo.class` 会连同 `Foo$*.class` 一起处理
- 新产物中不存在、但原 JAR 中属于同一 group 的旧内部类会被移除
- 与 patch 无关的资源文件会被保留
- 如果原 JAR 带签名，patch 后会自动移除失效的 `META-INF/*.SF`、`*.RSA`、`*.DSA`
- patch 总会写出报告文件：
  - `<output-jar>.report.json`
  - `<output-jar>.report.txt`

典型流程：

```bash
# 1. 先解包/反编译并修改源码（示例略）

# 2. 在外部工程里重新编译受影响类
javac -cp libs/*:app.jar -d build/classes/java/main src/com/example/Foo.java

# 3. 仅把编译结果回写到原 JAR
go run ./cmd/jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes/java/main \
  --output-jar app.patched.jar
```

终端和 patch report 会输出：

- 替换了哪些 class group
- 删除了哪些陈旧内部类
- 是否移除了失效签名文件
- 本次是否为 dry-run
- patch 执行耗时

## 配置文件

工具会从当前工作目录开始向上查找可选的 `config.yaml`，用于提供环境相关的默认值。

```yaml
jadx_path: /path/to/jadx
cfr_path: /path/to/cfr
default_retry_concurrency: 4
```

优先级如下：

1. CLI 显式参数
2. `config.yaml`
3. 内建默认值

仓库提供了 `config.yaml.example` 作为配置模板，可以复制后按本机环境修改：

```bash
cp config.yaml.example config.yaml
```

## 输出

反编译结果会写入目标输出目录，并附带：

- `sources/`：最终 Java 源码输出目录，包含保留的 `jadx` 结果和被 `cfr` 覆盖后的 retry 项
- `resources/`：`jadx` 产出的资源目录
- `report.json`
- `report.txt`

`report.json` / `report.txt` 中还会包含更丰富的诊断摘要：

- `retryCandidates`：进入 `cfr` 回退的 top-level class 数
- `totalElapsedMillis`：整次反编译总耗时
- `retryElapsedMillis`：retry 阶段耗时
- `retryOutcome`：某个 class 在回退阶段的最终结果，例如 `ambiguous_retry_output`、`invalid_retry_output`

## 项目结构

```text
cmd/jardec/             CLI 入口
internal/cli/           命令行参数与配置
internal/decompiler/    jadx / cfr 执行层
internal/jar/           JAR 清单与提取
internal/merge/         回退结果覆盖合并
internal/patch/         JAR patching 与归档重写
internal/pipeline/      主流程编排
internal/report/        报告模型与输出
```
