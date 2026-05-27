# syntax=docker/dockerfile:1.7
FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY agent_executor.py __main__.py __init__.py ./
EXPOSE 8080
ENV PORT=8080
CMD ["python", "__main__.py"]
