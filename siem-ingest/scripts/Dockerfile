FROM python:3.13-alpine
WORKDIR /app
COPY update-threatlist.py entrypoint.sh ./
RUN chmod +x entrypoint.sh
ENTRYPOINT ["./entrypoint.sh"]
