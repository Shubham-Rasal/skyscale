import json
from typing import Any

from skyscale.app import App


def _strip_skyscale_decorators(source: str) -> str:
    """Remove skyscale imports, SDK construction lines, and decorator lines
    so handler code runs without the SDK installed in the daemon venv."""
    lines = []
    for line in source.splitlines():
        stripped = line.lstrip()

        # Drop decorator lines: @app.xxx or @skyscale.xxx
        if stripped.startswith("@") and (
            "app." in stripped or stripped.startswith("@skyscale")
        ):
            continue

        # Drop import lines that pull in the skyscale SDK itself
        if stripped.startswith("import skyscale") or stripped.startswith("from skyscale"):
            continue

        # Drop module-level SDK construction:
        #   app = skyscale.App(...)
        #   image = skyscale.Image.pip_install(...)
        #   image = skyscale.Image(...)
        # These are always unindented (no leading whitespace).
        if not line.startswith(" ") and not line.startswith("\t"):
            if "skyscale.App(" in stripped or "skyscale.Image" in stripped:
                continue

        lines.append(line)
    return "\n".join(lines).strip() + "\n"


def build_deployment_spec(app: App, file_source: str) -> list[dict[str, Any]]:
    """Return one DeploymentSpec dict per web endpoint in the app.

    file_source is the full raw text of the user's .py file. We send the whole
    file (minus skyscale decorator lines) as the handler code so that
    module-level imports and helper functions are always available, and so we
    never have to call inspect.getsource() on importlib-loaded objects (which
    raises TypeError on Python 3.12+).
    """
    clean_source = _strip_skyscale_decorators(file_source)
    specs: list[dict[str, Any]] = []

    for fn_name, handle, web_meta in app.web_endpoints():
        is_cls = hasattr(handle, "_cls")

        if is_cls:
            enter_method = next(
                (
                    n
                    for n, m in handle._cls.__dict__.items()
                    if callable(m) and getattr(m, "_is_enter", False)
                ),
                None,
            )
            entry_type = "cls"
            entry_class = handle._cls.__name__
            entry_method = fn_name.split(".", 1)[1]
        else:
            enter_method = None
            entry_type = "function"
            entry_class = None
            entry_method = handle._fn.__name__

        image = handle.image
        hw_type = "gpu" if handle.gpu else "cpu"
        specs.append(
            {
                "app_name": app.name,
                "endpoint_name": fn_name,
                "entry_type": entry_type,
                "entry_class": entry_class or "",
                "entry_method": entry_method,
                "enter_method": enter_method or "",
                "code": clean_source,
                "requirements": image.to_requirements_txt() if image else "",
                "setup_script": image.to_setup_script() if image else "",
                "hardware": {
                    "type": hw_type,
                    "gpu_model": (handle.gpu or "") if hw_type == "gpu" else "",
                    "memory": handle.memory,
                    "cpu_target": getattr(handle, "cpu", "firecracker"),
                },
                "web": dict(web_meta),
                "scaledown_window": getattr(handle, "scaledown_window", 60),
            }
        )
    return specs


def main() -> None:
    """CLI entry: read module path from argv, print JSON specs."""
    import importlib.util
    import sys

    if len(sys.argv) < 2:
        print("usage: python -m skyscale._deploy <app.py>", file=sys.stderr)
        sys.exit(2)

    path = sys.argv[1]

    # Read the raw source before loading so we can send it verbatim.
    try:
        with open(path, "r") as fh:
            file_source = fh.read()
    except OSError as exc:
        print(f"cannot read {path}: {exc}", file=sys.stderr)
        sys.exit(1)

    spec = importlib.util.spec_from_file_location("_skyscale_app", path)
    if spec is None or spec.loader is None:
        print("failed to load module", file=sys.stderr)
        sys.exit(1)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)

    app = next(
        (v for v in vars(mod).values() if isinstance(v, App)),
        None,
    )
    if app is None:
        print("no skyscale.App instance found in module", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(build_deployment_spec(app, file_source)))


if __name__ == "__main__":
    main()
