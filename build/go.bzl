"""Go 服务便捷宏。

目的：收敛"Go 二进制 + 库 + OCI 镜像"的三件套样板，减少重复。

Usage:
    load("//build:go.bzl", "go_service")

    go_service(
        name = "server",
        srcs = ["main.go"],
        importpath = "github.com/yishuiliunian/nexusapi/backend/cmd/server",
        deps = ["//backend/internal/interface/http"],
        exposed_ports = ["8080/tcp"],
        repository = "ghcr.io/yishuiliunian/nexusapi-server",
    )

会自动生成：
    :{name}_lib       go_library
    :{name}           go_binary
    :{name}_image     OCI 镜像（dev）
    :{name}_image_prod OCI 镜像（prod）
    :{name}_image_push 推送（若 repository 非空）
"""

load("@rules_go//go:def.bzl", "go_binary", "go_library")
load("//build:oci.bzl", "go_oci_image")

def go_service(
        name,
        srcs,
        importpath,
        deps = [],
        exposed_ports = [],
        env = {},
        repository = None,
        visibility = ["//visibility:public"]):
    """创建 Go 服务的 library + binary + OCI image 三件套。"""

    go_library(
        name = name + "_lib",
        srcs = srcs,
        importpath = importpath,
        deps = deps,
        visibility = visibility,
    )

    go_binary(
        name = name,
        embed = [":" + name + "_lib"],
        visibility = visibility,
    )

    go_oci_image(
        name = name + "_image",
        binary = ":" + name,
        env = env,
        exposed_ports = exposed_ports,
        repository = repository,
        visibility = visibility,
    )
