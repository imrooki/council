# 统一构建入口：版本号从这里算，别手敲。
#
# 版本 = <VERSION 文件>+git.<提交短哈希>[.dirty]
#   例：0.2.0+git.c011cbd      干净的工作区
#       0.2.0+git.c011cbd.dirty 有未提交改动
#
# 为什么这么做：以前靠手敲「0.2.0-1025g / 1025h / 1025i」，
# 既看不出对应哪份源码，也分不清谁构建的。带上提交哈希之后，
# 任何一份二进制都能反查到具体源码，脏工作区也不会被误当成正式构建。
#
# VERSION 文件只在里程碑时手改；日常构建不动它。
[CmdletBinding()]
param(
    # 输出路径，默认装到 PATH 上那个位置。
    [string]$Out = '%USERPROFILE%\bin\council.exe'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $base = (Get-Content VERSION -Raw).Trim()
    if (-not $base) { throw 'VERSION 文件是空的' }

    $hash = (git rev-parse --short HEAD).Trim()
    if (-not $hash) { throw '拿不到 git 提交哈希：这个仓库还没有提交？' }

    # 只有在仓库里有改动时才标 dirty。未跟踪文件也算——一个没进版本库的
    # 源文件同样意味着这份二进制无法用这次提交复现。
    $status = git status --porcelain
    $version = "$base+git.$hash"
    if ($status) { $version = "$version.dirty" }

    Write-Host "构建 $version -> $Out"
    go build -ldflags "-X main.version=$version" -o $Out ./cmd/council
    if ($LASTEXITCODE -ne 0) { throw "go build 失败（exit $LASTEXITCODE）" }

    & $Out --version
} finally {
    Pop-Location
}
