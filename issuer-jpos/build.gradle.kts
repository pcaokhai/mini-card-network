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
    testImplementation(platform(libs.junit.bom))
    testImplementation("org.junit.jupiter:junit-jupiter")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    testImplementation(libs.assertj)
    testImplementation(libs.archunit)
}

application { mainClass = "org.jpos.q2.Q2" }
tasks.named<JavaExec>("run") { workingDir = file("src/dist") }
distributions { main { contents { from("src/dist") } } }
tasks.test { useJUnitPlatform() }
spotless { java { googleJavaFormat(); target("src/**/*.java") } }
