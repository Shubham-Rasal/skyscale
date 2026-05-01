from setuptools import setup, find_packages

setup(
    name="skyscale-sandbox",
    version="0.1.0",
    description="Python SDK for Skyscale (sandbox API + App deploy SDK)",
    packages=find_packages(include=["skyscale", "skyscale.*", "skyscale_sandbox", "skyscale_sandbox.*"]),
    python_requires=">=3.8",
    install_requires=[
        "requests>=2.28.0",
    ],
)
