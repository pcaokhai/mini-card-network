plugins {
    application
    id("com.diffplug.spotless")
}

java { toolchain { languageVersion = JavaLanguageVersion.of(25) } }
repositories { mavenCentral() }
dependencyLocking { lockAllConfigurations() }

dependencies {
    implementation(libs.jpos)
    implementation(libs.javalin)
    implementation(libs.micrometer.prometheus)
    implementation(libs.logback)
    implementation(libs.logstash.encoder)
    implementation(libs.postgresql)
    implementation(libs.hikaricp)
    implementation(libs.flyway.postgresql)
    testImplementation(platform(libs.junit.bom))
    testImplementation("org.junit.jupiter:junit-jupiter")
    testImplementation("org.junit.jupiter:junit-jupiter-params")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    testImplementation(libs.assertj)
    testImplementation(libs.archunit)
    testImplementation(libs.testcontainers.postgresql)
    testImplementation(libs.testcontainers.junit)
}

val generatePackager = tasks.register<GeneratePackagerTask>("generatePackager") {
    specFile.set(layout.projectDirectory.file("../contracts/iso8583/packager-spec.yaml"))
    outputFile.set(layout.projectDirectory.file("src/dist/cfg/iso87ascii.xml"))
}
tasks.named("compileJava") { dependsOn(generatePackager) }
tasks.named("test") { dependsOn(generatePackager) }

application { mainClass = "org.jpos.q2.Q2" }
tasks.named<JavaExec>("run") {
    workingDir = file("src/dist")
    // `make seed` overrides mainClass to run SeedMain instead of the Q2 server (ponytail: a
    // gradle run override, not a second Q2 deploy descriptor - seed never touches production).
    project.findProperty("mainClass")?.let { mainClass.set(it as String) }
}
// The application plugin already merges src/dist into the distribution by convention;
// declaring it again here made distTar/distZip see every file twice.
tasks.test { useJUnitPlatform() }
spotless { java { googleJavaFormat(); target("src/**/*.java") } }
