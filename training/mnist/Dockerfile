FROM nvidia/cuda:12.1.0-runtime-ubuntu22.04

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ curl python3 python3-pip \
    && ln -sf python3 /usr/bin/python \
    && ln -sf pip3 /usr/bin/pip \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY train.py /app/train.py

ENV JOB_ID=""
ENV EXECUTION_ID=""
ENV CONTROL_PLANE_URL="http://localhost:8080"
ENV EPOCHS="5"
ENV BATCH_SIZE="256"
ENV LR="1e-3"

EXPOSE 8082

CMD ["python", "/app/train.py"]
