"""Minimal Skyscale App with one web endpoint (CPU / POST)."""

import skyscale

app = skyscale.App("demo-web")

image = skyscale.Image.pip_install("pandas", "numpy")


@app.function(image=image)
@skyscale.web_endpoint(method="POST", path="/process")
def process(data: dict) -> dict:
    import pandas as pd

    df = pd.DataFrame(data)
    return {"result": df.describe().to_dict()}
