FROM python:3.14-slim
RUN pip install --no-cache-dir \
    'python-lsp-server[rope,pyflakes]==1.15.0' \
    'rope==1.15.2' 'autopep8==2.3.2' 'python-lsp-isort==0.2.1'
ENV HOME=/tmp PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1 XDG_CACHE_HOME=/tmp/cache
COPY warm.py /opt/warm.py
COPY fast_server.py /opt/fast_server.py
RUN python3 /opt/warm.py && rm /opt/warm.py
RUN python3 /opt/fast_server.py --help > /dev/null
COPY test_fast_server.py /opt/test_fast_server.py
RUN python3 /opt/test_fast_server.py && rm /opt/test_fast_server.py
COPY --chmod=0555 start.sh /usr/local/bin/start-analyzer
WORKDIR /work
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/start-analyzer"]

CMD []
