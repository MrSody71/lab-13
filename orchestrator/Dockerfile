FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
ENV PYTHONUNBUFFERED=1
ENV OTEL_SERVICE_NAME=orchestrator
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
CMD ["python", "main.py"]
