# cf-probe - Cloudflare IP 优选测速工具

轻量级、跨平台的 Cloudflare 优选 IP 测速工具，支持命令行和网页双界面。

## 核心特性

- 基于 XIU2/CloudflareSpeedTest 内核，支持延迟 + 下载速度双重测速
- 支持全球 97 个 Cloudflare 数据中心，可按地区筛选
- 原生支持 IP:端口 格式，适配反代场景
- 提供 Web 界面和命令行两种模式
- CSV 下载、反代列表导出及 cfnew/GitHub 上报仅包含下载速度达标的节点
- Web 结果计数显示下载速度达标节点数
- 内置 Cron 定时任务支持
- 提供 Android APK 和 Docker 镜像

## 快速开始

### 下载安装

从 [Releases](https://github.com/DaiZhouHui/cf-probe/releases) 下载对应平台版本：


**Windows:**
```bash
解压后直接运行 `CfProbe-Windows-Amd64.exe`
```




**adroid APK:**
```bash
下载安卓APK安装运行 `CfProbe-Android.apk`
```



**Linux/macOS:**
```bash
tar -xzf CfProbe-Linux-Amd64.tar.gz
chmod +x CfProbe-Linux-Amd64
./CfProbe-Linux-Amd64
```


**Docker(已弃用):**
```bash
docker run -d \
  --name cf-probe \
  -p 8080:8080 \
  -v $(pwd)/data:/data \
  ghcr.io/dazhouhui/cf-probe:latest
```

**从源码编译:**
```bash
git clone https://github.com/DaiZhouHui/cf-probe.git
cd cf-probe
go build -o yx ./cmd/yx
```

### Android APK 签名（项目维护者）

APK 发布签名文件不要提交到仓库。首次设置时，在本机 PowerShell 生成一把 JKS 密钥（alias 固定为 `yx`，按提示设置新密码；不要复用已经发到聊天或提交到仓库的密码）：

```powershell
keytool -genkeypair -v -keystore "$HOME\cf-probe-release.jks" -storetype JKS -alias yx -keyalg RSA -keysize 2048 -validity 10000
```

将密钥文件内容转为 Base64 并复制：

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$HOME\cf-probe-release.jks")) | Set-Clipboard
```

在 GitHub 仓库的 **Settings → Secrets and variables → Actions → New repository secret** 中添加：

- `ANDROID_KEYSTORE_BASE64`：粘贴刚才复制的 Base64 内容。
- `KEYSTORE_PASS`：生成密钥时设置的新密码。

保存后清空剪贴板（`Set-Clipboard -Value ''`）。GitHub Actions 会从 Secret 还原签名文件并构建 APK；不要把 `.jks` 文件或密码提交到仓库。丢失签名密钥后，使用新密钥签名的 APK 不能覆盖安装旧密钥签名的版本。

## 使用方法

### Web 界面

```bash
./yx
# 访问 http://127.0.0.1:8080
```

自定义监听地址:
```bash
./yx web -listen 0.0.0.0:8080
```

### 命令行测速

基础用法:
```bash
# 测速 10 个 IP，速度下限 1MB/s
./yx test -n 10 -sl 1

# 仅测试香港和新加坡节点
./yx test -colo HKG,SIN -n 20

# 启用 IPv6
./yx test -ipv6 -n 15

# 只测延迟（更快）
./yx test -nodl -n 50
```

高级参数:
```bash
./yx test \
  -n 30 \           # 测速数量
  -sl 2 \           # 速度下限 2MB/s
  -tl 200 \         # 延迟上限 200ms
  -t 500 \          # 并发线程数
  -dt 15 \          # 单 IP 测速时长 15 秒
  -mt 300 \         # 总时长上限 5 分钟
  -o result.csv     # 输出文件名
```

使用自定义 IP 列表:
```bash
./yx test -f custom_ips.txt -n 20
```

### 结果上报

上报到 cfnew API:
```bash
./yx test -n 10 -upload api -domain your.domain.com -uuid YOUR_UUID -clear
```

推送到 GitHub:
```bash
./yx test -n 10 -upload github -repo username/repo -token ghp_xxx -path results/ip_list.txt -clear
```

单独执行上报:
```bash
./yx upload api -domain your.domain.com -uuid YOUR_UUID -limit 20 -clear
```

生成反代列表:
```bash
./yx proxy -limit 20
```

### 定时任务

添加定时任务:
```bash
./yx cron -add "test -n 10 -sl 2 -upload api -clear" -at "0 */6 * * *"
```

查看任务:
```bash
./yx cron
```

移除任务:
```bash
./yx cron -remove
```

## 参数说明

### 测速参数 (test)

| 参数 | 说明 | 默认值 |
|------|------|--------|
| -colo | 机场代码，逗号分隔（如 HKG,SIN,NRT） | 全部 |
| -ipv6 | 启用 IPv6 测速 | false |
| -n | 测速 IP 数量 | 10 |
| -sl | 下载速度下限（MB/s） | 1 |
| -tl | 平均延迟上限（ms） | 1000 |
| -t | 并发线程数 | 200 |
| -port | 测速端口 | 443 |
| -url | 自定义测速 URL | 内置默认 |
| -f | 自定义 IP 文件路径 | 自动下载 |
| -nodl | 仅测延迟 | false |
| -dt | 单 IP 测速时长（秒） | 10 |
| -mt | 总时长上限（秒），0 无限制 | 0 |
| -o | 结果输出文件名 | result.csv |

### 上报参数

| 参数 | 说明 | 必填 |
|------|------|------|
| -upload | 上报方式：api 或 github | 是 |
| -domain | cfnew Worker 域名 | API 模式 |
| -uuid | cfnew UUID | API 模式 |
| -repo | GitHub 仓库（owner/repo） | GitHub 模式 |
| -token | GitHub Token | GitHub 模式 |
| -path | 仓库内文件路径 | cloudflare_ips.txt |
| -limit | 上报 IP 数量上限 | 10 |
| -clear | 上报前清空数据 | false |

### Web 参数 (web)

| 参数 | 说明 | 默认值 |
|------|------|--------|
| -listen | 监听地址 | 127.0.0.1:8080 |
| -no-open | 禁止自动打开浏览器 | false |

### 定时任务参数 (cron)

| 参数 | 说明 |
|------|------|
| -add | 添加定时任务 |
| -at | Cron 表达式 |
| -remove | 移除所有任务 |

## 文件说明

| 文件名 | 说明 |
|--------|------|
| result.csv | 完整测速结果 |
| ips_ports.txt | 反代专用列表（IP:端口格式） |
| yx-config.json | 配置文件 |
| Cloudflare.txt | IPv4 IP 段缓存 |
| Cloudflare_ipv6.txt | IPv6 IP 段缓存 |
| yx-cron.log | 定时任务日志 |

数据存储位置优先级：当前目录 > 程序目录 > ~/.cf-probe > 临时目录

可通过环境变量 YX_DATA_DIR 指定存储路径。

## 常见问题

**Q: macOS 提示无法验证开发者？**

前往系统设置 -> 隐私与安全性，点击仍要打开。

**Q: Linux 权限不足？**

```bash
chmod +x CfProbe-Linux-Amd64
```

**Q: 测速结果不理想？**

- 增加 -n 参数测试更多 IP
- 降低 -sl 速度下限
- 提高 -tl 延迟上限
- 使用 -colo 指定优质节点（如 HKG,NRT,SIN）

## 相关项目
- [byJoey/yx-tools](https://github.com/byJoey/yx-tools)  -基于该项目二次修改
- [XIU2/CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest) - 测速内核来源

## 开源协议

MIT License
