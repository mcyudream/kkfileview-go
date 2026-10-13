> **状态（2026-10-13）：已全量实现，并在 Ubuntu 实体机上完成真实文件全链路实测（Office→PDF、PDF→JPG、TIF 双模式、压缩包、加密文档、媒体转码、前端渲染类 25 类预览页面逐一比对）。实测结果与修正记录见 [README.md](README.md) 末节。**
# kkFileView Go 重构可行性评估

> 评估对象：[kekingcn/kkFileView](https://github.com/kekingcn/kkFileView) @ `3b82689`（v5.0.3 线，2026-10-08）
> 评估日期：2026-10-13 ｜ 源码副本：`../kkfileview-src`（供后续逐函数对照）
> 核心约束：**接口完全一致，可无感替换**（同一批 URL、同一份配置、同一个 Docker 端口，前端零改动）

## 一、结论

**可行。** kkFileView 是一个"薄服务端 + 厚前端 + 重外部进程"的架构：Java 只做路由分发、URL 编解码、缓存、安全过滤和转换编排，真正的活全交给 LibreOffice（Office→PDF）、前端 JS 库（pdf.js / SheetJS / cadviewer / ofd.js / cornerstone 等，与语言无关、可直接拷贝）和外部命令（ffmpeg / cadviewer 转换包）。

服务端 90% 以上的逻辑是 HTTP 路由、字符串处理、缓存和安全检查，这些映射到 Go 是一一对应的；真正的风险集中在 **3 个转换链路的保真度**（下文第四节），且都有明确的替代方案。

## 二、"无感替换"到底要保住什么（验收清单）

以下是从源码逐 Controller / Filter / Config / Template 提取的完整对外接口面。**Go 版必须逐项字节级一致。**

### 2.1 HTTP 接口（12 个 + 静态 + 健康检查）

| 路径 | 方法 | 参数（* = 必填） | 返回 |
|---|---|---|---|
| `/` `/index` | GET | — | 首页 HTML |
| `/record` `/sponsor` `/integrated` `/contact` | GET | — | 对应页面 HTML |
| `/onlinePreview` | GET | `url*`、`encryption`、`key`、`highlightall=false`、`page=0`、`kkagent=false`；另支持藏在 url 里的 `fullfilename`、`kkCompressfileKey`、`kkCompressfilepath` 与请求参数 `officePreviewType`、`forceUpdatedCache`、`usePasswordCache`、`tifPreviewType`、`filePassword`，header `kk-proxy-authorization` | 预览页 HTML，**模板名由文件类型决定（25 种）** |
| `/picturesPreview` | GET | `urls*`（`\|` 分隔）、`currentUrl`（独立 base64）、`encryption`、`key` | 图片组 HTML |
| `/getCorsFile` | GET | `urlPath*`、`encryption`、`key` | 文件二进制流（pdf.js 跨域代理，含 FTP 分支） |
| `/addTask` | GET | `url*`、`encryption`、`key` | **纯文本** `"success"` 或错误消息 |
| `/fileUpload` | POST | `file`(multipart)*、`path` | `{msg, code}` JSON |
| `/createFolder` | POST | `path`、`folderName*` | JSON |
| `/deleteFile` | POST | `fileName`(base64)*、`password*` | JSON |
| `/deleteFile/captcha` | 任意 | — | 验证码 JPEG |
| `/listFiles` | POST | `path`、`searchText`、`page=0`、`size=20`、`sort`、`order` | `{total, displayedCount, totalInDirectory, data[], page, size, totalPages, ...}` JSON |
| `/actuator/health` | GET | — | 健康检查 JSON |
| `/**` | GET | — | `static/` 全部资源 **+ `file.dir` 目录下的全部转换产物**（PDF/JPG 从这里被 pdf.js 和 img 标签访问） |

### 2.2 URL 编码约定（接入方约定，必须逐字节兼容）

- `url` 参数 = `Base64(url)` 后再 `urlEncode`；**Base64 里的空格必须还原成 `+`**、去掉换行（这是历史接入方共知的坑）。
- `encryption=aes` 时 = `AES/ECB/PKCS5Padding`（key 固定 16 位，默认 `1234567890123456`），先 Base64 解码再 AES 解密。
- 解码后校验协议头 `^(https|http|ftp|rtsp|mms|file)://`，不合法时**返回字符串 `base641`**（而非报错），由上层渲染错误页——这个错误码语义必须保留。
- 中文文件名在 url 路径段的"半编码"传递规则（`encodeURLFileName` + `+→%20`、`%3A→:` 等六个替换）散落在 4 处工具方法里，是穿越/XSS 修复的产物，需逐函数照搬。

### 2.3 请求过滤器链（顺序敏感，安全语义必须一致）

```
ChinesePathFilter(UTF-8) → TrustHostFilter → TrustDirFilter
→ BaseUrlFilter(baseUrl 决策) → UrlCheckFilter(// 和尾/重定向) → SecurityFilterProxy(禁 TRACE)
→ AttributeSetFilter(给模板注入 ~20 个属性)
```
- `TrustHostFilter`：默认**拒绝所有**未配置 host；`trust.host=*` 放行；支持 `*.example.com` 通配、`192.168.*` IPv4 通配、CIDR（只解析字面量 IP、不做 DNS，防 rebinding）。
- `baseUrl` 决策链：header `X-Base-Url` > 配置 `base.url` > 动态拼接 `scheme://host:port/contextPath/`。所有模板里的绝对 URL 都由它拼出。

### 2.4 配置面（约 100 个键，全部 `${KK_XXX:default}` 环境变量可覆盖）

`application.properties`（在 jar 内 `server/src/main/config/`）+ `ConfigConstants` 的 `@Value` 默认值，两处默认值**不一致时以代码为准**（例如 `pdf2jpg.dpi` 文件里是 144、代码里 105）。Go 版配置加载必须：读同一文件、同一环境变量名、同一默认值。关键项：`server.port=KK_SERVER_PORT(8012)`、`server.servlet.context-path=KK_CONTEXT_PATH`、`file.dir=KK_FILE_DIR`、`cache.type=jdk|redis|default(rocksdb)`、`cad.conversionmodule`、`office.preview.type=pdf|image|allImages`、`trust.host`、`delete.password`、`watermark.*` 等。

### 2.5 前端产物（零改动拷贝）

`resources/static/` 下 25 个第三方前端库（pdfjs、xspreadsheet、xlsx 前端解析、cadviewer、ofd、epub、drawio、bpmn、cornerstone-dcm、xmind、msg viewer…）与 31 个 `.ftl` 模板 —— **与 Java 无关，原样拷贝**。模板需翻译成 Go 模板语法，但产出的 HTML 必须一致（DOM 结构、id/class、JS 变量名、URL 拼接方式全不能变，前端 JS 依赖这些）。

## 三、模块映射表（Java → Go）

| Java 组件 | 职责 | Go 方案 | 风险 |
|---|---|---|---|
| Spring Boot + Jetty + Filter | HTTP/路由/过滤器 | `net/http` + 自实现中间件链 | 低 |
| Freemarker × 31 | 模板渲染 | `html/template` 翻译移植 | 低（`?js_string` 转义需手工等价，见下） |
| `WebUtils`/`KkFileUtils`/`FileHandlerService` | URL 编解码、FileAttribute 组装 | 纯 Go 字符串函数照搬 | 低 |
| 3 种 Cache 实现（JDK map / Redisson / RocksDB） | 转换缓存 | `sync.Map` / `go-redis` / `bbolt` | 低 |
| TrustHost/TrustDir 过滤器 | 白名单/CIDR/防穿越 | 纯 Go 照搬 | 低 |
| `DownloadUtils`/`HttpRequestUtils` | 下载源文件（basic auth、UA、ignoreSSL、重定向策略） | `net/http` Transport | 低 |
| FTP 预览（commons-net） | FTP 流式读取 | `net/ftp`（stdlib） | 低 |
| 验证码 | 删除验证码 | `golang.org/x/image/font` 自绘 | 低 |
| jodconverter + LibreOffice | Office→PDF | **池化 exec `soffice --headless --convert-to`**，或常驻 listener 讲 UNO（重，不推荐） | **中** |
| PDFBox | PDF→JPG（智能 DPI）、页数、加密 PDF | `pdftoppm`（poppler）或 `go-pdfium`(cgo)；页数用 pdfcpu | **中** |
| **aspose-cad（商业库）** | CAD→SVG/PDF | 改用 `cadviewer` 转换包或 ODA File Converter（外部进程）；`cad.conversionmodule` 语义保留 | **中高** |
| Apache POI | 检测加密 Office、密码兼容性 | 自写 OLE2/CFB 解析或 soffice 预处理探测 | 中 |
| sevenzipjbinding + junrar | 7z / rar 解压 | `7zz` CLI 或 pure-Go 解 rar | 中 |
| JavaMail | .eml 解析 | `net/mail`（stdlib） | 低 |
| POI-HSMF | .msg 解析 | 自写 MSG(CFB) 解析 | 中 |
| jai-imageio | tif/heic/webp 解码 | `x/image` + 备用 ImageMagick | 低 |
| cpdetector | 编码探测 | `saintfish/chardet` | 低 |
| Quartz/`@Scheduled` | 缓存定时清理 | `robfig/cron` | 低 |
| Actuator | `/actuator/health` | 手写同构 JSON | 低 |
| Freemarker 的 `?js_string` | JS 字符串转义 | **Go 无等价物，需实现 Java `js_string` 语义**（`"` `\` `/` `'` `>` `<` `&` 的转义序列与 PDF.js 侧解码严格对应） | 中 – 踩坑点 |

**注意：** OFD 前端化（服务端不转换，ofd.js 渲染）——这是 v5 的一个关键变化，Go 版 OFD 只需照搬 CommonPreviewImpl + 模板即可。

## 四、三大转换链路的保真策略

### 4.1 Office→PDF（jodconverter → soffice CLI）

Java 侧通过 UNO 的 FilterData 传 6 个转换参数：`EncryptFile`、`PageRange`、`Watermark`、`Quality`、`MaxImageResolution`、`ExportBookmarks`/`ExportNotes`/`DocumentOpenPassword`。
Go 侧两条路：
1. **CLI filter options**（推荐）：LO 7.x 支持 `soffice --convert-to 'pdf:writer_pdf_Export:{"ExportBookmarks":{"type":"boolean","value":true},"Quality":{"type":"long","value":80}}'`，多数参数可透传；
2. 文档密码（load 属性 `Password`）CLI 不支持，需要 Basic 宏驱动 `loadComponentFromURL`。可在 Go 侧预生成 `~/.config/libreoffice/4/user/basic/Standard/Module1.xba`，headless 执行。这是加密 Office 文档的唯一路径，需单独立项验证。

原版用常驻 2 个 soffice（端口 2001/2002）+ 任务队列，CLI 方案按进程池 + `office.plugin.task.timeout` 5 分钟超时 + 每进程 200 任务重启来对齐其行为。

### 4.2 PDF→JPG（PDFBox → pdftoppm）

原版智能 DPI：0–50 页 150，51–100 页 120，101–200 页 96，>200 页 72。`pdftoppm -r <dpi> -jpeg` 逐页输出即可 1:1 复刻（缓存命名 `<prefix>/<i>.jpg` 与原版一致，前端按索引取图）。
加密 PDF 缓存（`office.documentopenpasswords=true` 时给转换产物加回密码）：用 `qpdf --encrypt` 或 pdfcpu 完成。
Docker 镜像加 `poppler-utils` 即可，与原镜像同为 Debian base，体积影响小。

### 4.3 CAD（aspose-cad → 外部转换器）

`cad.conversionmodule` 原语义：1=aspose（Java 内联）、2/3=cadviewer 外部包。Go 版没有 aspose，**直接把默认值切到 ODA File Converter / cadviewer CLI**，`cad.cadconverterpath` 配置语义保留。会损失 aspose 特有的一部分格式（IGES/STEP/BREP/3DM 等），需要列一张格式兼容表与原版贴出的支持列表逐项 diff —— 这是唯一可能对外可见能力缩水的点，需在 README 明示或以 ODA + FreeCAD CLI 组合补齐。

## 五、分期路线（建议顺序，每期可独立验证）

1. **P0 骨架与"页面 0 差异"**：配置加载（同文件/同 env/同默认值）→ 过滤器链 → `/onlinePreview` 分发 → FileType 全量映射 → 31 模板移植 → 拷贝 static/。验收：与原版并排对 25 类文件同 URL 返回的 HTML 做 diff。
2. **P1 前端渲染类**（pdf/纯文本/代码/md/json/xml/svg/eml/msg/drawio/bpmn/xmind/dcm/epub/media/ofd/picture/picturesPreview）：只做下载 + 缓存 + 模板，零转换。
3. **P2 Office 链路**：下载、缓存、异步等待页状态机、`soffice` CLI 池化、PdfBox→pdftoppm、加密文档、水印。
4. **P3 CAD/TIF/媒体转码**：外部转换器接入、ffmpeg。
5. **P4 文件管理面**：上传/建目录/删除/列表/验证码/addTask/缓存清理 cron/actuator。
6. **P5 安全与边缘**：TrustHost 全量语义、FTP、AES、basic auth、XSS、path traversal 全量用例（原版 tests/ 有 4 个安全测试类可直接搬成 Go 测试）+ Dockerfile + 中文文件名/乱码回归。

## 六、风险与未决点（需要在 P1 前验证的 3 件事）

1. **模板转义等价性**：Freemarker `?js_string` ↔ Go 自定义转义函数，错了 pdf.js 的 file= 参数就错（中文文件名必踩）。
2. **soffice CLI 的 FilterData 透传完整度**：书签/批注/页码范围/水印逐项对拍。
3. **aspose-cad 缺位后的格式清单**：与官方支持列表 diff，决定补 ODA 还是明示缩水。

其余均有成熟 Go 生态对应物，工作量主要是"翻译"而不是"发明"。粗估全量对照实现约为数人周级别（P0–P1 占一半以上工时），不建议一次性全上，按 P0→P5 增量交付、每期用 diff/回归保证"无感"。

## 七、替代方案对照（如果全量重写风险不可接受）

- **JNI/Exe 边界方案**：Go 做全部 HTTP 面，Java 侧只保留 3 个转换 Worker（Office/CAD/PDFBox）以子进程通信 —— 丧失单二进制的意义，不推荐。
- **渐进替换**：Go 版先承载前端渲染类（P1，约占请求量多数），Office/CAD 回落反代到原版服务 —— 可以作为灰度上线的过渡策略。
