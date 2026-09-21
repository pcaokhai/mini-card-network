rootProject.name = "issuer-jpos"
pluginManagement {
    val spotlessVersion: String by settings
    plugins { id("com.diffplug.spotless") version spotlessVersion }
}
