# jardec

一个基于 Go 的命令行工具，用于对 JAR 执行 **jadx 优先、Procyon 回退** 的反编译流程。

## 当前能力

- 用 `jadx` 对整个 JAR 做首轮反编译
- 以 **top-level class** 作为完整性基线
- 对可疑结果执行严格判定，包含 `JADX WARN`
- 对失败项逐类使用 Procyon 补偿
- 允许 Procyon 覆盖 retry 项的 `jadx` 输出
- 生成 `report.json` 和 `report.txt`
- 将已编译的 `.class` 变更按 top-level class group 打回原 JAR
- 支持 patch dry-run、显式 class group 选择和持久化 patch 报告
- 对 patch replacement 做 class identity 校验，并识别 unchanged / no-op patch
- 在反编译报告中输出 retry 候选数、retry 结果和耗时摘要

## 依赖

- Go 1.24+
- 本地可用的 `jadx`
- 本地可用的 Procyon
- 本地可用的 `javac`（仅 `patch-sources` 需要）

## 安装 `jadx` 和 Procyon

### 安装 `jadx`

推荐直接使用官方发布包，下载后解压，并把 `bin/jadx` 加到 `PATH`，或者把绝对路径写进 `config.yaml`。

```bash
# 例如：下载并解压后
export PATH="/path/to/jadx/bin:$PATH"

# 验证
jadx --version
```

如果你的系统包管理器提供了 `jadx`，也可以直接用包管理器安装，但建议优先确认版本是否符合预期。

### 安装 Procyon

Procyon 以 jar 形式发布，`jardec` 会通过 `java -jar` 直接调用它。你可以直接把 `procyon.jar` 的绝对路径写进 `config.yaml` 或通过 `--procyon-path` 指定。

```bash
# 从 GitHub Releases 下载最新版本
# https://github.com/mstrobel/procyon/releases
wget https://github.com/mstrobel/procyon/releases/download/v0.6.0/procyon-decompiler-0.6.0.jar -O /path/to/procyon.jar

# 验证
java -jar /path/to/procyon.jar --help
```

如果你希望 `jardec` 从 `$PATH` 自动发现 Procyon，可以准备一个本地包装脚本：

```bash
#!/usr/bin/env bash
exec java -jar /path/to/procyon.jar "$@"
```

保存为例如 `/usr/local/bin/procyon` 后，赋予可执行权限：

```bash
chmod +x /usr/local/bin/procyon
```

## 常用命令

```bash
make test
make build
make run
```

## 直接运行

```bash
go run ./cmd/jardec decompile \
  --input sample.jar \
  --output out \
  --jadx-path /path/to/jadx \
  --procyon-path /path/to/procyon.jar \
  --classpath libs \
  --classpath third_party/extra.jar
```

常用参数：

- `--input`：输入 JAR 路径
- `--output`：输出目录
- `--jadx-path`：`jadx` 可执行文件路径
- `--procyon-path`：Procyon jar 路径，`jardec` 会通过 `java -jar` 调用
- `--classpath`：为反编译追加依赖 JAR 或依赖目录；当前保证用于 Procyon fallback retry，可重复指定
- `--temp-dir`：临时目录根路径
- `--keep-temp`：保留中间工作目录
- `--retry-concurrency`：Procyon 回退并发数

全局参数：

- `--config`：指定 `config.yaml` 路径（默认从当前目录向上搜索 `config.yaml`）

如果项目里经常复用同一组依赖，可以把默认反编译依赖写进 `config.yaml`：

```yaml
decompile_classpath:
  - libs
  - third_party/extra.jar
```

`decompile` 的 classpath 顺序约定为：

1. 输入 JAR（仅 Procyon retry 自动加入）
2. `config.yaml` 的 `decompile_classpath`
3. 命令行上的 `--classpath`

重复项会按首次出现顺序去重。`config.yaml` 里的相对路径会按 **该配置文件所在目录** 解析。

如果某个 classpath entry 指向目录，`jardec` 会：

1. **只读取该目录的直接子项**
2. 只收集其中扩展名为 `.jar`（大小写不敏感）的文件
3. 按文件名稳定排序后并入最终 classpath
4. 忽略更深层的嵌套子目录

如果目录里没有任何 JAR，命令会直接报错，而不是静默忽略这个目录。

> 当前已保证 **Procyon fallback retry** 使用上述 classpath。JADX CLI 对外部依赖的支持能力有限，因此本版本不会承诺 `--classpath` 一定会影响首轮 JADX 整包反编译。

`jardec` 现在采用显式子命令：

- `jardec decompile ...`：执行反编译
- `jardec patch-classes ...`：把已编译的 `.class` 增量打回原 JAR
- `jardec patch-sources ...`：先编译少量 Java 源码，再复用 patch 流程回写原 JAR

## Patch 已编译 class 回原 JAR

`patch-classes` 用于把已经重新编译好的 `.class` 文件增量写回原始 JAR。它仍然是底层的 **compiled-class-only** 入口：如果你已经有确定的 `.class` 输出，直接用它最简单。

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
- 会校验每个 replacement `.class` 的内部 binary name 与归档路径一致，避免误把错误产物打进包
- 可用 `--class` 精确限制只处理某些 top-level binary class name
- 可用 `--dry-run` 先预览哪些 group 会真正变更、哪些只是 unchanged，因此不会改写归档
- 以 **top-level class group** 作为替换单元，例如 `Foo.class` 会连同 `Foo$*.class` 一起处理
- 新产物中不存在、但原 JAR 中属于同一 group 的旧内部类会被移除
- 与 patch 无关的资源文件会被保留
- 对已存在的 class entry，会尽量保留原 ZIP metadata，并在原 group 位置附近写回，减少 patched JAR 的结构漂移
- 如果所有目标 group 都 unchanged，apply 会保留原 JAR 字节级内容，不会重建归档，也不会移除签名文件
- 如果原 JAR 带签名，只有在本次 patch 真的改动归档内容时，才会移除失效的 `META-INF/*.SF`、`*.RSA`、`*.DSA`
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

- 哪些 class group 被判定为 `changed` 或 `unchanged`
- 本次是否为 no-op
- 删除了哪些陈旧内部类
- 失效签名文件是被移除还是被保留
- 本次是否为 dry-run
- patch 执行耗时

## Patch 已编辑 Java 源码回原 JAR

`patch-sources` 适合“只改了少量 `.java`，想先编译这些 target，再把结果回写原包”的场景。它不会尝试重建整个工程；v1 只支持 **显式 target + 显式 classpath** 的受控编译。

命令格式：

```bash
go run ./cmd/jardec patch-sources \
  --input-jar app.jar \
  --sources-dir edited-src \
  --output-jar app.patched.jar \
  --class com.example.Foo \
  --javac-path /path/to/javac \
  --classpath libs/dependency-a.jar \
  --classpath libs/dependency-b.jar
```

行为约定：

- 必须至少提供一个 `--class`，并按 top-level binary class name 指定，例如 `com.example.Foo`
- `--class` 只接受 top-level class；像 `com.example.Foo$Inner` 这样的 nested target 会直接报错
- `--sources-dir` 需要满足稳定映射：`com.example.Foo` 对应 `edited-src/com/example/Foo.java`
- 编译时会自动把 `--input-jar` 加入 classpath，再把 `config.yaml` 里的 `patch_sources_classpath` 依次接在后面，最后再追加每个 `--classpath`
- 编译产物会先写入隔离的临时 classes 目录，再复用现有 `patch-classes` patch 逻辑
- 如果 `javac` 失败，或成功退出但没有产生某个 target 对应的 top-level `.class`，命令会在改写归档前失败
- `patch-sources` 同样会写出 `<output-jar>.report.json` 和 `<output-jar>.report.txt`，其中会额外记录 compile 阶段状态和诊断

和 `patch-classes` 的区别：

- `patch-classes`：你自己准备好 `.class`，`jardec` 只负责 patch
- `patch-sources`：`jardec` 先用 `javac` 编译指定 `.java`，再复用同一套 patch 语义

如果项目里经常复用同一组依赖，可以把默认编译环境写进 `config.yaml`：

```yaml
javac_path: /path/to/javac
patch_sources_classpath:
  - libs/dependency-a.jar
  - libs/dependency-b.jar
```

优先级和拼接顺序是：

1. 输入 JAR（自动加入）
2. `config.yaml` 的 `patch_sources_classpath`
3. 命令行上的 `--classpath`

其中重复项会按首次出现顺序去重，命令行仍然可以用于一次性的追加依赖。`config.yaml` 里的相对路径会按 **该配置文件所在目录** 解析，因此从子目录执行命令时也能稳定找到同一组依赖。

## 配置文件

工具会从当前工作目录开始向上查找可选的 `config.yaml`，用于提供环境相关的默认值。

```yaml
jadx_path: /path/to/jadx
procyon_path: /path/to/procyon.jar
decompile_classpath:
  - libs
  - third_party/extra.jar
javac_path: /path/to/javac
patch_sources_classpath:
  - libs/dependency-a.jar
  - libs/dependency-b.jar
default_retry_concurrency: 4
```

优先级如下：

1. CLI 显式参数
2. `config.yaml`
3. 内建默认值

说明：

- `patch_sources_classpath` 仅用于 `patch-sources`
- `decompile_classpath` 用于 `decompile` 的 Procyon fallback retry
- `decompile_classpath` 的每一项既可以是单个 JAR，也可以是包含多个 JAR 的目录；目录只做**非递归**展开
- `config.yaml` 中的相对路径按该配置文件所在目录解释

仓库提供了 `config.yaml.example` 作为配置模板，可以复制后按本机环境修改：

```bash
cp config.yaml.example config.yaml
```

## 输出

反编译结果会写入目标输出目录，并附带：

- `sources/`：最终 Java 源码输出目录，包含保留的 `jadx` 结果和被 Procyon 覆盖后的 retry 项
- `resources/`：`jadx` 产出的资源目录
- `report.json`
- `report.txt`

`report.json` / `report.txt` 中还会包含更丰富的诊断摘要：

- `retryCandidates`：进入 Procyon 回退的 top-level class 数
- `totalElapsedMillis`：整次反编译总耗时
- `retryElapsedMillis`：retry 阶段耗时
- `retryOutcome`：某个 class 在回退阶段的最终结果，例如 `ambiguous_retry_output`、`invalid_retry_output`
- `dependencyWarnings`：额外记录诸如 `Could not load the following classes` 这类依赖缺失降质信号；它们不会单独改变 success/failure 判定

## 项目结构

```text
cmd/jardec/             CLI 入口
internal/cli/           命令行参数与配置
internal/decompiler/    jadx / Procyon 执行层
internal/jar/           JAR 清单与提取
internal/merge/         回退结果覆盖合并
internal/patch/         JAR patching 与归档重写
internal/pipeline/      主流程编排
internal/report/        报告模型与输出
internal/sourcepatch/   Java source compile + patch 编排
```
