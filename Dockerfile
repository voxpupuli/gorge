FROM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62

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
  CMD curl -f http://localhost:8080/readyz || exit 1

ENTRYPOINT ["/gorge"]
CMD [ "serve" ]
