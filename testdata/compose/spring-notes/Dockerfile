FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY pom.xml .
COPY src src
RUN --mount=type=cache,target=/root/.m2 mvn -q -B -DskipTests package

FROM eclipse-temurin:21-jre
COPY --from=build /src/target/notes-0.0.1.jar /app.jar
ENTRYPOINT ["java","-jar","/app.jar"]
