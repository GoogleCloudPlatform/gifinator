FROM alpine

COPY ./gopath/bin/frontend /frontend
COPY ./gopath/bin/gifcreator /gifcreator
COPY ./gopath/bin/render /render

COPY ./frontend/static /static
COPY ./frontend/templates /templates

COPY ./gifcreator/scene /scene

# Add trusted CA root bundles
RUN   apk update \
  &&   apk add ca-certificates wget \
  &&   update-ca-certificates

ENV FRONTEND_TEMPLATES_DIR=/templates
ENV FRONTEND_STATIC_DIR=/static
ENV SCENE_PATH=/scene
