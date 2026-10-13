# kkFileView Go 重写版

[kekingcn/kkFileView](https://github.com/kekingcn/kkFileView)（v5.0.3 线）的 Go 语言重写，**对外接口完全一致**，可直接无感替换原 Java 版。

## 特性一致性

- **相同的 HTTP 接口**：`/onlinePreview`、`/picturesPreview`、`/getCorsFile`、`/addTask`、`/fileUpload`、`/createFolder`、`/deleteFile`、`/deleteFile/captcha`、`/listFiles`、`/index` 等页面路由、`/actuator/health`，以及 `/**` 静态资源（`static/` 与 `file.dir` 转换产物直连）。
- **相同的 URL 约定**：`url` 参数 Base64 + urlEncode 双编码（Base64 内空格还原为 `+`）、`encryption=aes`（AES/ECB/PKCS5Padding）、错误码 `base641` 语义。
- **相同的过滤器链与安全语义**：TrustHost（默认拒绝 + 通配符 + IPv4 通配 + CIDR）、TrustDir（file 协议目录限制）、URL 规范化重定向、UTF-8 编码、禁 TRACE。
- **相同的配置面**：约 100 个配置键与 `${KK_XXX:default}` 环境变量（`KK_SERVER_PORT=8012`、`KK_FILE_DIR`、`KK_TRUST_HOST` 等）逐一对应，读取同一份 `application.properties`。
- **相同的 HTML 产出**：上游 `web/` 下 **31 个 Freemarker 模板 + 3 个 html 文件原样使用**（`templates/` 与上游 `diff -r` 零差异），内置微型 FreeMarker 兼容引擎（`internal/ftl`）逐字节渲染，保证前端零改动。
- **相同的文件分类映射**：FileType 扩展名→处理器映射表（含静态块赋值顺序）与缓存命名规则逐一对照移植。

## 与原版的外部依赖差异

| 能力 | 原版 | Go 版 |
|---|---|---|
| Office→PDF | jodconverter + LibreOffice UNO | LibreOffice CLI（池化 profile，FilterData JSON 透传；加密文档走 Basic 宏） |
| PDF→JPG | PDFBox | poppler `pdftoppm`（智能 DPI 策略一致） |
| 加密 PDF 缓存 | PDFBox | `qpdf` |
| CAD 转换 | aspose-cad（商业库） | cadviewer / ODA File Converter CLI，LibreOffice Draw 兜底 |
| 压缩包 | sevenzipjbinding/junrar | `7zz/7z` CLI，Go 标准库 zip/tar 兜底 |
| 视频转码 | ffmpeg | ffmpeg |
| 缓存 | JDK map / Redisson / RocksDB | sync.Map / go-redis / bbolt |
| TIF→PDF | ImageMagick | 纯 Go PDF 写出（JPEG DCTDecode 内嵌，多页 xref），ImageMagick 兜底 |

> CAD 默认不使用 aspose-cad，`cad.conversionmodule` 语义保留；格式覆盖度以 cadviewer/ODA 为准，这是唯一可能对外可见的能力缩水点。

## 构建与运行

```bash
go build -o kkfileview .
./kkfileview -config config/application.properties -templates templates -static static
```

环境变量与原版一致：`KK_SERVER_PORT`、`KK_CONTEXT_PATH`、`KK_FILE_DIR`、`KK_TRUST_HOST`、`KK_OFFICE_HOME`、`KK_CACHE_TYPE`（`jdk`/`redis`/`default`）等。

### Docker

**直接拉取（推荐）**：

```bash
docker pull registry.yudream.online/library/kkfileview-go:latest
docker run -d -p 8012:8012 -v /data/kkfileview:/opt/kkfileview/file \
  -e KK_TRUST_HOST='*' registry.yudream.online/library/kkfileview-go:latest
```

已发布标签：`latest`、`5.0.3`（linux/amd64）。镜像内已预装 LibreOffice、poppler-utils、p7zip-full、ffmpeg、qpdf、文泉驿中文字体，转换产物落在卷 `/opt/kkfileview/file`。常用环境变量：`KK_TRUST_HOST`（默认拒绝所有未配置 host，`*` 放行）、`KK_FILE_DIR`、`KK_SERVER_PORT`、`KK_CACHE_TYPE`。

启动后访问 `http://<ip>:8012/` 即为原版首页，URL 输入框可直接贴文件地址试预览。

**本地构建**：仓库提供两个 Dockerfile，按网络环境二选一：

| 文件 | 适用 | 说明 |
|---|---|---|
| `Dockerfile` | 能访问 Docker Hub | 标准多阶段构建（golang 构建 + debian slim 运行） |
| `Dockerfile.registry` | 内网/无 Docker Hub | 基于 `registry.yudream.online/library/kkfileview:5.0.2`，交叉编译二进制 COPY 进去 |

内网环境一键构建并推送（`scripts/build-image.sh`，需要仓库 robot 账号密码）：

```bash
REG_PASS='<robot-push 密码>' scripts/build-image.sh 5.0.3
```

> 注意：该私有仓库前置 nginx 的 `client_max_body_size` 约 1MB，`docker push` 的单体 PUT 会被 413 拒绝，脚本改走 `tools/pushimg` 以 512KB 分块 PATCH 上传（跨 base 层已存在则直接 HEAD 跳过）。

## 目录结构

```
main.go                    入口
config/application.properties   与原版一致的配置
internal/
  ftl/       微型 FreeMarker 引擎（含回归测试，tplcheck 全量校验）
  config/    配置加载（${ENV:default} 展开、动态刷新）
  model/     FileType / FileAttribute / ReturnResponse
  utils/     WebUtils/KkFileUtils/Download/FTP/编码探测/验证码/转换状态机/AES
  cache/     jdk / redis / bbolt 三种缓存实现
  convert/   soffice 池化、pdftoppm、CAD、TIF、ffmpeg、压缩包
  handler/   25 个文件预览处理器 + FileHandlerService
  server/    过滤器链、控制器、静态资源、会话、缓存清理
templates/   上游 Freemarker 模板（未翻译，零 diff）
static/      上游前端静态资源（原样拷贝）
cmd/tplcheck 模板渲染校验工具
scripts/build-image.sh        内网环境构建并推送镜像到私有仓库
tools/pushimg                 分块推送镜像工具（绕开仓库 nginx 1MB 单次请求上限）
Dockerfile / Dockerfile.registry  两种构建路径（Docker Hub / 内网基础镜像）
```

## 测试

```bash
go test ./internal/...     # 单元测试
go run ./cmd/tplcheck      # 31 模板全量渲染回归
```

单元测试覆盖：全量模板解析渲染、`?js_string`/`?url` 转义语义、空白剥离、条件/循环/序列字面量/默认值运算符、URL 编解码往返、信任主机匹配（通配符/CIDR）、路径包含判断。

## 真实环境实测（2026-10-13，Ubuntu 24.04 + LibreOffice 25.x + poppler + 7z + ffmpeg）

在 Ubuntu 实体机（`192.168.100.142`）上部署 Go 二进制 + 同款 `templates/`/`static/`/`application.properties`，以真实文件走完整链路，逐项对上原版模板标题与产物：

| 链路 | 结果 |
|---|---|
| docx/pptx/long.docx → PDF | 转换成功，页面 `PDF预览`，产物 `testdocx.pdf`/`testpptx.pdf`/`longdocx.pdf` |
| xlsx（web 模式）/csv | `test.xlsx预览` / `test.csv预览`，前端解析页正常 |
| pdf / long.pdf | `PDF预览`，pdf.js viewer 正常加载 |
| Office 图片模式（`officePreviewType=image`） | 等待页→`图片预览`，`longdocx/0..4.jpg` 五张，智能 DPI 与 `<n>.jpg` 命名同原版 |
| encrypted.docx + `filePassword` | Basic 宏链路解密转换成功，出 PDF |
| album.zip / with-password.zip | 解包 + ztree 树形 JSON 正确，压缩包预览页正常 |
| multi.tif / pic.tif（`tif.preview.type=tif`） | 多帧 TIFF→多张 JPG，无跑飞（纯 Go 解码只取一帧） |
| multi.tif（`tif.preview.type=pdf`） | 纯 Go PDF 写出成功（`%PDF-1.4`，3131B，2 页），`PDF预览` + `pdfUrl=multitif.pdf` |
| mp4 直连 | `sample.mp4播放器`，ckplayer `video:` 指向源 URL |
| avi（`media.convert.disable=false`，发货默认） | 与原版一致返回"系统还不支持该格式文件的在线预览" |
| avi（`media.convert.disable=true`） | 下载→异步 ffmpeg 转码→等待页→`movie.avi播放器`（产物 `movieavi.mp4`） |
| txt / md / xml / json | `普通文本预览` / `文本预览` / `XML预览器` / `JSON文件预览`，textData base64 内容逐字节比对一致 |
| eml / msg | 页面对应模板正确渲染（由前端 JS 解析） |
| jpg / png | `图片预览` |
| `/getCorsFile` 跨域代理 | 返回 16749B PDF 流，pdf.js 跨域读取正常 |
| AES（`encryption=aes`）URL | 解密、预览链路正常 |
| 403/404 语义、URL 重定向、TrustHost/TrustDir | 与 Java 版一致 |

### 实测中修正的问题

- **媒体预览类型分支**：原版 `FileType` 把 `avi/mov/wmv/mkv/3gp/rm` 映射为 `MEDIACONVERT`（非 `MEDIA`），`media.convert.disable=false`（仓库默认）时 http 源的 avi 会走"不支持"分支。Go 版最初无条件返回播放页，已按源码修回一致语义。
- **TIF→PDF 产物命名**：一度怀疑与 Java 的 `cacheName` 构造不同，逐行比对 `FileHandlerService.java:277` 后确认前缀算法 `substring(0,lastIndexOf(".")) + suffix + "."` 完全一致（`multi.tif` → `multitif.pdf`）。
- **图片模式缓存语义**：同一 `cacheName` 在 PDF 模式已入缓存后再请求图片模式，两版同样返回"Office转换缓存异常"——行为与 Java 一致，非缺陷。

### 已知限制

- **CAD 格式覆盖**：aspose-cad 换成 cadviewer/ODA CLI 后，IGES/STEP/BREP/3DM 等 aspose 专有格式需单独验证。
- **rar 解压**：依赖 `7zz` CLI，rar5 需 7-Zip 21+；rar 不装外部工具时不可用（原版 junrar 同理有格式上限）。
- **加密 Office 判定**：覆盖真实 MS-Office OOXML `EncryptionInfo` 与 OLE2 UTF-16 标记；LibreOffice 导出的"带密码导出"docx 不带 `EncryptionInfo` 结构，两版都无法据此识别。
- **`msg` 渲染**：与原版一致，只做下载+模板，正文解析在前端。
- **Excel 转 HTML（`office.type.web=web`）**：与原版一致走前端 xlsx 解析（SheetJS），服务端只传 URL。
