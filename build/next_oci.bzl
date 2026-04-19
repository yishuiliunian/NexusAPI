"""OCI image macros for Next.js frontend services.

This module provides shared macros for building OCI (Docker) images
for Next.js applications using rules_oci v2.x with js_image_layer.

Usage:
    load("//DevOps/BuildSystem/Server:next_oci.bzl", "next_oci_image")

    next_oci_image(
        name = "image",
        next_build = ":next",
        repository = "registry.example.com/my-app",
    )

Image Tags:
    When pushing to a remote registry, images are tagged with:
    - "latest" (always)
    - The value of --embed_label (if specified, typically commit SHA)

    To push with a specific version tag:
        bazel run //path:image_push --stamp --embed_label=abc123
"""

load("@aspect_bazel_lib//lib:expand_template.bzl", "expand_template")
load("@aspect_rules_js//js:defs.bzl", "js_image_layer")
load("@rules_oci//oci:defs.bzl", "oci_image", "oci_load", "oci_push")

def next_oci_image(
        name,
        next_build,
        base = "@node_slim",
        env = {},
        exposed_ports = ["3000/tcp"],
        labels = {},
        repository = None,
        visibility = ["//visibility:public"]):
    """Creates an OCI image for a Next.js application using js_image_layer.

    This macro creates:
    - Image layers via js_image_layer (app + node_modules)
    - An OCI image that runs the Next.js application
    - A load target for importing to local Docker daemon
    - (Optional) A push target for remote registry

    Args:
        name: The name of the image target.
        next_build: The next() macro target name (e.g., ":next").
            This macro will automatically use "{next_build}_start_binary" for packaging.
        base: The base image. Defaults to node:20-alpine.
        env: Environment variables to set in the image.
        exposed_ports: Ports to expose. Defaults to ["3000/tcp"].
        labels: OCI labels to add to the image.
        repository: Remote repository URL for pushing (optional).
        visibility: Visibility of the targets.
    """

    # Derive the js_binary target name from the next build target
    # next() macro creates {name}_start_binary for OCI packaging
    next_binary = next_build.lstrip(":") + "_start_binary"

    # Default environment for Next.js production
    default_env = {
        "NODE_ENV": "production",
        "PORT": "3000",
        "HOSTNAME": "0.0.0.0",
    }
    default_env.update(env)

    # Platform definition for Linux amd64 (CI/production)
    native.platform(
        name = name + "_linux_amd64",
        constraint_values = [
            "@platforms//os:linux",
            "@platforms//cpu:x86_64",
        ],
    )

    # Use js_image_layer to create layers with proper node_modules handling
    js_image_layer(
        name = name + "_layers",
        binary = ":" + next_binary,
        platform = ":" + name + "_linux_amd64",
        root = "/app",
        visibility = visibility,
    )

    # Get package name for cmd/workdir paths
    pkg = native.package_name()

    # Create the OCI image
    # Use the linux_amd64 specific base image since we only build for amd64
    oci_image(
        name = name,
        base = base + "_linux_amd64",
        tars = [":" + name + "_layers"],
        # Command to run the js_binary
        cmd = ["/app/" + pkg + "/" + next_binary],
        entrypoint = [""],
        env = default_env,
        exposed_ports = exposed_ports,
        labels = labels,
        # Workdir must be the runfiles directory
        workdir = "/app/" + pkg + "/" + next_binary + ".runfiles/_main",
        visibility = visibility,
    )

    # Create a load target for importing to local Docker daemon
    oci_load(
        name = name + "_tarball",
        image = ":" + name,
        repo_tags = ["nexusapi/" + name + ":latest"],
        visibility = visibility,
    )

    # Create a push target if repository is specified
    if repository:
        # Create stamped tags file for remote_tags
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
