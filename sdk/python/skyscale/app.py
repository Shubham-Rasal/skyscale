import dataclasses
from collections.abc import Callable, Iterator
from typing import Any, Optional


class App:
    def __init__(self, name: str):
        self.name = name
        self._functions: dict[str, "FunctionHandle"] = {}
        self._classes: dict[str, "ClsHandle"] = {}

    def function(
        self,
        image: Any = None,
        gpu: Optional[str] = None,
        cpu: str = "firecracker",
        memory: int = 256,
        timeout: int = 30,
    ):
        def decorator(fn: Callable[..., Any]) -> "FunctionHandle":
            handle = FunctionHandle(
                fn,
                image=image,
                gpu=gpu,
                cpu=cpu,
                memory=memory,
                timeout=timeout,
            )
            self._functions[fn.__name__] = handle
            return handle

        return decorator

    def cls(
        self,
        image: Any = None,
        gpu: Optional[str] = None,
        cpu: str = "firecracker",
        memory: int = 256,
        scaledown_window: int = 60,
    ):
        def decorator(cls_: type) -> "ClsHandle":
            handle = ClsHandle(
                cls_,
                image=image,
                gpu=gpu,
                cpu=cpu,
                memory=memory,
                scaledown_window=scaledown_window,
            )
            self._classes[cls_.__name__] = handle
            return handle

        return decorator

    def web_endpoints(self) -> Iterator[tuple[str, Any, dict[str, str]]]:
        for name, h in self._functions.items():
            if hasattr(h._fn, "_web_endpoint"):
                yield name, h, h._fn._web_endpoint
        for name, h in self._classes.items():
            for method_name, method in h.web_methods():
                yield f"{name}.{method_name}", h, method._web_endpoint


@dataclasses.dataclass
class FunctionHandle:
    _fn: Callable[..., Any]
    image: Any
    gpu: Optional[str]
    cpu: str
    memory: int
    timeout: int

    def remote(self, *args: Any, **kwargs: Any) -> Any:
        raise RuntimeError(
            "Call .remote() only after deploying with `skyscale deploy`"
        )

    def local(self, *args: Any, **kwargs: Any) -> Any:
        return self._fn(*args, **kwargs)

    def __call__(self, *args: Any, **kwargs: Any) -> Any:
        return self._fn(*args, **kwargs)


@dataclasses.dataclass
class ClsHandle:
    _cls: type
    image: Any
    gpu: Optional[str]
    cpu: str
    memory: int
    scaledown_window: int

    def web_methods(self) -> Iterator[tuple[str, Any]]:
        for name, attr in vars(self._cls).items():
            if callable(attr) and hasattr(attr, "_web_endpoint"):
                yield name, attr
