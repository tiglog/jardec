# jardec

一个 Go 命令行工具，对 JAR 执行 **jadx 优先、Procyon 回退** 的反编译流程。

## 快速上手

```bash
go run ./cmd/jardec decompile \
  --input app.jar \
  --output out \
  --jadx-path /path/to/jadx \
  --procyon-path /path/to/procyon.jar
```

反编译结果写入 `out/`，包含 `sources/`（Java 源码）、`resources/`（资源文件）和 `report.json` / `report.txt`。

如果项目有外部依赖 JAR，通过 `--classpath` 追加（Procyon 回退时需要它们来还原类型引用）：

```bash
go run ./cmd/jardec decompile \
  --input app.jar \
  --output out \
  --jadx-path /path/to/jadx \
  --procyon-path /path/to/procyon.jar \
  --classpath libs \
  --classpath third_party/dep.jar
```

`--classpath` 的条目可以是单个 JAR，也可以是目录（非递归展开其中所有 `*.jar`，按文件名排序）。

## 配置复用

把常用工具路径和依赖写进 `config.yaml`，避免每次敲长命令：

```yaml
jadx_path: /path/to/jadx
procyon_path: /path/to/procyon.jar
decompile_classpath:
  - libs
  - third_party/extra.jar
default_retry_concurrency: 4
```

[config.yaml.example](./config.yaml.example) 提供了完整模板，复制后修改即可：

```bash
cp config.yaml.example config.yaml
```

工具从当前目录向上搜索 `config.yaml`，也可通过 `--config` 指定路径。优先级：CLI 参数 > `config.yaml` > 内建默认值。

`decompile_classpath` 同时用于 `decompile` 的 Procyon 回退和 `patch-sources` 的 javac 编译，相对路径按配置文件所在目录解析。

## 依赖

- Go 1.24+
- `jadx` — [GitHub Releases](https://github.com/skylot/jadx/releases)，确保 `bin/jadx` 在 `$PATH` 或写进配置
- Procyon — 下载 jar 后用 `java -jar` 调用：
  ```bash
  wget https://github.com/mstrobel/procyon/releases/download/v0.6.0/procyon-decompiler-0.6.0.jar -O /path/to/procyon.jar
  ```
- `javac` — 仅 `patch-sources` 需要

## 完整参数

```
jardec decompile [选项]

  --input value, -i value          输入 JAR 路径
  --output value, -o value         输出目录
  --jadx-path value                jadx 可执行文件路径
  --procyon-path value             Procyon jar 路径
  --classpath value [--classpath]  追加依赖 JAR 或目录（可重复）
  --temp-dir value                 临时目录根路径
  --keep-temp                     保留中间工作目录（默认 false）
  --retry-concurrency value        回退并发数（默认 CPU 核数）
  --config value                   指定 config.yaml 路径
```

## 报告

反编译完成后输出目录包含：

- `sources/` — 最终 Java 源码（保留 jadx 输出，被 Procyon 成功恢复的类会被覆盖）
- `resources/` — jadx 产出的资源文件
- `report.json` / `report.txt` — 包含总数、回退候选数、成功恢复数、失败数、耗时及逐类详情

## 子命令一览

| 命令 | 用途 |
|---|---|
| `jardec decompile` | 反编译 JAR |
| `jardec patch-classes` | 把编译好的 `.class` 增量写回原 JAR |
| `jardec patch-sources` | 编译少量 `.java` 源码后写回原 JAR |

### patch-classes

```bash
go run ./cmd/jardec patch-classes \
  --input-jar app.jar \
  --classes-dir build/classes/java/main \
  --output-jar app.patched.jar
```

可选 `--dry-run` 预览，`--class` 限制只处理指定 top-level class group。以 group 为替换单元（含 `$` 内部类），旧内部类自动清理，资源文件保留。

### patch-sources

```bash
go run ./cmd/jardec patch-sources \
  --input-jar app.jar \
  --sources-dir edited-src \
  --output-jar app.patched.jar \
  --class com.example.Foo \
  --javac-path /path/to/javac \
  --classpath libs/dep.jar
```

`--sources-dir` 下文件按包路径组织（如 `com/example/Foo.java`）。编译后复用 `patch-classes` 的 patch 逻辑。

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

## 开发

```bash
make test    # 运行测试
make build   # 编译到 bin/jardec
make run     # 显示帮助
```
