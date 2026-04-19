"""Wrapper macro to make the Next.js rules more ergonomic.

Based on the official bazelbuild/examples pattern.
See: https://github.com/bazelbuild/examples/tree/main/frontend/next.js
"""

load("@aspect_rules_js//js:defs.bzl", "js_binary", "js_run_binary", "js_run_devserver")

def next(
        name,
        srcs,
        data,
        next_js_binary,
        next_bin,
        next_build_out = ".next",
        **kwargs):
    """Generates Next.js build, dev & start targets.

    `{name}`       - a js_run_binary build target that runs `next build`
    `{name}_dev`   - a js_run_devserver binary target that runs `next dev`
    `{name}_start` - a js_run_devserver binary target that runs `next start`

    Args:
        name: The name of the build target.

        srcs: Source files to include in build & dev targets.
            Typically these are ts_project targets from source folders
            such as `app`, `components`, `lib`, etc.

        data: Data files to include in all targets.
            These are typically npm packages required for the build & configuration files such as
            package.json and next.config.ts.

        next_js_binary: The next js_binary. Used for the `build` target.

        next_bin: The next bin command. Used for the `dev` and `start` targets.
            Typically the path to the next entry point from the current package,
            e.g., `./node_modules/.bin/next`.

        next_build_out: The `next build` output directory. Defaults to `.next`.

        **kwargs: Other attributes passed to all targets such as `tags`.
    """

    tags = kwargs.pop("tags", [])

    # `next build` creates an optimized bundle of the application
    # https://nextjs.org/docs/app/api-reference/cli/build
    js_run_binary(
        name = name,
        tool = next_js_binary,
        args = ["build"],
        srcs = srcs + data,
        out_dirs = [next_build_out],
        chdir = native.package_name(),
        mnemonic = "NextJsBuild",
        progress_message = "Building Next.js application",
        tags = tags,
        **kwargs
    )

    # `next dev` runs the application in development mode
    # https://nextjs.org/docs/app/api-reference/cli/dev
    js_run_devserver(
        name = "{}_dev".format(name),
        tool = next_js_binary,
        args = ["dev"],
        data = srcs + data,
        chdir = native.package_name(),
        grant_sandbox_write_permissions = True,
        tags = tags,
        **kwargs
    )

    # `next start` runs the application in production mode
    # https://nextjs.org/docs/app/api-reference/cli/start
    js_run_devserver(
        name = "{}_start".format(name),
        tool = next_js_binary,
        args = ["start"],
        data = data + [name],
        chdir = native.package_name(),
        tags = tags,
        **kwargs
    )

    # `{name}_start_binary` - a js_binary target for OCI image packaging
    # This is required because js_image_layer needs js_binary, not js_run_devserver
    # We use a local start.js entry point that requires next/dist/bin/next
    js_binary(
        name = "{}_start_binary".format(name),
        entry_point = "start.js",
        data = data + [name],
        chdir = native.package_name(),
        tags = tags,
        **kwargs
    )
