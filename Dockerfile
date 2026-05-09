FROM ubuntu:latest

WORKDIR /app

COPY scheduler /app/scheduler
COPY web /app/web

ENV TODO_PORT=7540
ENV TODO_DBFILE=/data/scheduler.db

EXPOSE 7540
VOLUME ["/data"]

CMD ["/app/scheduler"]
