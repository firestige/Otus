# syntax=docker/dockerfile:1
FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

# HEP UDP listener port
EXPOSE 9060/udp

# Unbuffered output so logs are immediate in docker compose
CMD ["python", "-u", "app.py"]
