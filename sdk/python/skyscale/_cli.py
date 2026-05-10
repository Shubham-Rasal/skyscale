"""CLI entry point: `skyscale run <file.py>`"""

import argparse
import sys


def main():
    parser = argparse.ArgumentParser(
        prog="skyscale",
        description="Skyscale serverless GPU platform CLI",
    )
    sub = parser.add_subparsers(dest="command")

    run_parser = sub.add_parser("run", help="Submit a training script as a GPU job")
    run_parser.add_argument("file", help="Python training script to run")
    run_parser.add_argument("--function", default=None, help="Function name inside the script (unused, reserved)")
    run_parser.add_argument("--gpu", default="a100", help="GPU model (default: a100)")
    run_parser.add_argument("--image", required=True, help="Docker image for the training container")
    run_parser.add_argument("--url", default=None, help="Control plane URL (overrides SKYSCALE_API_URL)")
    run_parser.add_argument("--epochs", type=int, default=None)
    run_parser.add_argument("--batch-size", type=int, default=None, dest="batch_size")
    run_parser.add_argument(
        "--env", action="append", default=[], metavar="KEY=VALUE",
        help="Extra env vars (repeatable)"
    )

    args = parser.parse_args()

    if args.command == "run":
        from skyscale._run import run  # noqa: PLC0415

        extra_env: dict[str, str] = {}
        for kv in args.env:
            if "=" not in kv:
                print(f"[skyscale] invalid --env value: {kv!r} (expected KEY=VALUE)", file=sys.stderr)
                sys.exit(1)
            k, v = kv.split("=", 1)
            extra_env[k] = v

        run(
            args.file,
            function=args.function,
            gpu=args.gpu,
            image=args.image,
            env=extra_env or None,
            control_plane_url=args.url,
            epochs=args.epochs,
            batch_size=args.batch_size,
        )
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
