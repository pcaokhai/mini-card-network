FROM eclipse-temurin:25-jdk AS build
WORKDIR /src
# build.gradle.kts's generatePackager task reads ../contracts/iso8583/packager-spec.yaml
# relative to the issuer-jpos project dir, so the build context is the repo root (see
# compose.yaml), not issuer-jpos/ alone - contracts/ must land as a sibling of issuer-jpos/.
COPY contracts contracts
COPY issuer-jpos issuer-jpos
WORKDIR /src/issuer-jpos
RUN ./gradlew --no-daemon installDist

FROM eclipse-temurin:25-jre
WORKDIR /app
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar /otel/opentelemetry-javaagent.jar
COPY --from=build /src/issuer-jpos/build/install/issuer-jpos/ /app/
EXPOSE 8000 8081 9465
ENV JAVA_TOOL_OPTIONS="-javaagent:/otel/opentelemetry-javaagent.jar"
ENTRYPOINT ["/app/bin/issuer-jpos"]
