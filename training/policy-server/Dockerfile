FROM vllm/vllm-openai:latest

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY serve.py .

ENV MODEL_NAME=Qwen/Qwen3-0.6B
ENV PORT=8000

EXPOSE 8000

CMD ["python", "serve.py"]
