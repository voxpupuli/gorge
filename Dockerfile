FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

# Create non-root user and set up permissions in a single layer
RUN adduser -k /dev/null -u 10001 -D gorge \
  && chgrp 0 /home/gorge \
  && chmod -R g+rwX /home/gorge

# Copy application binary with explicit permissions
COPY --chmod=755 gorge /

# Set working directory
WORKDIR /home/gorge

# Switch to non-root user
USER 10001

# Define volume
VOLUME [ "/home/gorge" ]

# Set health check
HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget -qO- --spider http://localhost:8080/readyz 2>/dev/null || wget -qO- --spider --no-check-certificate https://localhost:8080/readyz 2>/dev/null

ENV GORGE_BIND=0.0.0.0
ENTRYPOINT ["/gorge"]
CMD [ "serve" ]
