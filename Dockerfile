# kkfileview-go 构建镜像
FROM golang:1.26-alpine AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /kkfileview .

# 运行镜像：Debian + LibreOffice + poppler + p7zip + ffmpeg
FROM debian:bookworm-slim

ENV KKFILEVIEW_BIN_FOLDER=/opt/kkfileview/bin \
    TZ=Asia/Shanghai \
    LANG=zh_CN.UTF-8 \
    LANGUAGE=zh_CN:zh \
    LC_ALL=zh_CN.UTF-8

RUN apt-get update && apt-get install -y --no-install-recommends \
        libreoffice-writer libreoffice-calc libreoffice-impress libreoffice-draw \
        libreoffice-java-common \
        poppler-utils \
        p7zip-full \
        ffmpeg \
        fonts-noto-cjk \
        ca-certificates \
        curl \
        tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /opt/kkfileview/bin

COPY --from=build /kkfileview /opt/kkfileview/bin/kkfileview
COPY config/ /opt/kkfileview/bin/config/
COPY templates/ /opt/kkfileview/bin/templates/
COPY static/ /opt/kkfileview/bin/static/

VOLUME /opt/kkfileview/file
EXPOSE 8012

WORKDIR /opt/kkfileview/bin
CMD ["/opt/kkfileview/bin/kkfileview", "-config", "/opt/kkfileview/bin/config/application.properties", "-templates", "/opt/kkfileview/bin/templates", "-static", "/opt/kkfileview/bin/static"]
