From alpine:3.3
Maintainer Ethan J. Jackson

RUN VER=1.7 \
&& export GOROOT=/tmp/build/go GOPATH=/tmp/build/gowork \
&& export PATH=$PATH:$GOROOT/bin \
&& apk update \
&& apk add --no-cache ca-certificates git --virtual .build_deps \
&& apk add --no-cache iproute2 \
&& mkdir -p /var/run/netns \
# Alpine uses musl instead of glibc which confuses go.
# They're compatible, so just symlink
&& mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 \
&& mkdir /tmp/build && cd /tmp/build \
&& wget https://storage.googleapis.com/golang/go$VER.linux-amd64.tar.gz \
&& gunzip -c go$VER.linux-amd64.tar.gz | tar x \
&& go get -u github.com/NetSys/quilt \
&& cd $GOPATH/src/github.com/NetSys/quilt \
&& git checkout paper \
&& go test github.com/NetSys/quilt/... \
&& cd /tmp/build \
&& go install github.com/NetSys/quilt \
&& cp $GOPATH/bin/* /\
&& git -C "$GOPATH/src/github.com/NetSys/quilt/" show --pretty=medium --no-patch \
	> /buildinfo \
&& rm -rf /tmp/build \
&& apk del .build_deps
