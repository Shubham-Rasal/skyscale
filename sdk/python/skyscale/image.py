class Image:
    def __init__(self):
        self._pip: list[str] = []
        self._apt: list[str] = []
        self._run: list[str] = []

    @classmethod
    def pip_install(cls, *packages: str) -> "Image":
        img = cls()
        img._pip = list(packages)
        return img

    def apt_install(self, *packages: str) -> "Image":
        self._apt.extend(packages)
        return self

    def run_commands(self, *cmds: str) -> "Image":
        self._run.extend(cmds)
        return self

    def to_requirements_txt(self) -> str:
        return "\n".join(self._pip)

    def to_setup_script(self) -> str:
        lines = []
        if self._apt:
            lines.append(
                "apt-get update && apt-get install -y " + " ".join(self._apt)
            )
        lines.extend(self._run)
        return "\n".join(lines)
