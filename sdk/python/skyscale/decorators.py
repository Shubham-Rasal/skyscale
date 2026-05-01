def enter():
    """Marks a class method as the container init hook (runs once on boot)."""

    def decorator(fn):
        fn._is_enter = True
        return fn

    return decorator


def web_endpoint(method: str = "POST", path: str = "/"):
    """Marks a function or class method as an HTTPS web endpoint."""

    def decorator(fn):
        fn._web_endpoint = {"method": method.upper(), "path": path}
        return fn

    return decorator


def method():
    """Marks a class method as a remotely callable method (non-web)."""

    def decorator(fn):
        fn._is_method = True
        return fn

    return decorator
