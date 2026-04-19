"""Go 服务的 OCI 镜像打包宏。

目标：统一把 Go 二进制打成可部署的 OCI 镜像，最终推到 ghcr.io。

设计简化（对比 AIO 的内部 registry 双镜像方案）：
- 单一基础镜像 distroless/static（~2MB，静态 Go 二进制所需极简运行时）
- 不维护 prod 变体（YAGNI，个人项目）

Usage:
    load("//build:oci.bzl", "go_oci_image")

    go_oci_image(
        name = "image",
        binary = ":server",
        exposed_ports = ["8080/tcp"],
        repository = "ghcr.io/yishuiliunian/nexusapi-server",
    )

构建目标：
- :{name}          OCI 镜像
- :{name}_tarball  导入到本地 Docker 守护进程（bazel run ... -- --norun 不合适；直接 bazel run）
- :{name}_push     （需要 repository）推送到 ghcr.io；标签为 latest + BUILD_EMBED_LABEL

推送：
    bazel run //path:image_push --stamp --embed_label=$(git rev-parse --short HEAD)
"""

load("@aspect_bazel_lib//lib:expand_template.bzl", "expand_template")
load("@rules_oci//oci:defs.bzl", "oci_image", "oci_load", "oci_push")
load("@rules_pkg//pkg:tar.bzl", "pkg_tar")

def go_oci_image(
        name,
        binary,
        base = "@distroless_static",
        env = {},
        exposed_ports = [],
        labels = {},
        repository = None,
        visibility = ["//visibility:public"]):
    """把一个 go_binary 打成 OCI 镜像。

    Args:
        name: 目标名。同时派生 {name}_tarball、{name}_push（若 repository 非空）。
        binary: 目标 go_binary 的 label（如 ":server"）。
        base: 基础镜像。默认 @distroless_static（需在 MODULE.bazel 中 oci.pull 过）。
        env: 容器环境变量。
        exposed_ports: 暴露端口列表，如 ["8080/tcp"]。
        labels: OCI labels。
        repository: 可选的远端仓库地址。为空则不生成 push 目标。
        visibility: 目标可见性。
    """

    binary_name = binary.split(":")[-1]

    # 把 binary 放到 /app/{name} 的一层 tar
    pkg_tar(
        name = name + "_layer",
        srcs = [binary],
        package_dir = "/app",
    )

    oci_image(
        name = name,
        base = base,
        tars = [":" + name + "_layer"],
        entrypoint = ["/app/" + binary_name],
        env = env,
        exposed_ports = exposed_ports,
        labels = labels,
        user = "nonroot",
        workdir = "/app",
        visibility = visibility,
    )

    # 导入到本地 Docker daemon
    oci_load(
        name = name + "_tarball",
        image = ":" + name,
        repo_tags = ["nexusapi/" + binary_name + ":latest"],
        visibility = visibility,
    )

    if repository:
        # --embed_label=xxx 会被替换到 0.0.0 占位符中
        expand_template(
            name = name + "_tags",
            out = name + "_tags.txt",
            template = ["latest", "0.0.0"],
            stamp_substitutions = {
                "0.0.0": "{{BUILD_EMBED_LABEL}}",
            },
        )

        oci_push(
            name = name + "_push",
            image = ":" + name,
            repository = repository,
            remote_tags = ":" + name + "_tags",
            visibility = visibility,
        )
