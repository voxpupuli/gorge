FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

ARG TARGETPLATFORM

# Create non-root user and set up permissions in a single layer
RUN adduser -k /dev/null -u 10001 -D gorge \
  && chgrp 0 /home/gorge \
  && chmod -R g+rwX /home/gorge

# Copy application binary with explicit permissions
COPY --chmod=755 $TARGETPLATFORM/gorge /

# Set working directory
WORKDIR /home/gorge

# Switch to non-root user
USER 10001

# Define volume
VOLUME [ "/home/gorge" ]

ENV GORGE_BIND=0.0.0.0
ENTRYPOINT ["/gorge"]
CMD [ "serve" ]
