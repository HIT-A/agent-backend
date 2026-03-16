FROM python:3.11-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    libmagic1 \
    poppler-utils \
    tesseract-ocr \
    libreoffice-writer \
    libreoffice-impress \
    pandoc \
    && rm -rf /var/lib/apt/lists/*

# Set environment
ENV PYTHONUNBUFFERED=1

# Create app directory
WORKDIR /app

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy MCP server code
COPY server.py .

# Run the MCP server
CMD ["python", "-u", "server.py"]