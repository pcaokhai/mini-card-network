package io.mcn.settlement;

import static com.tngtech.archunit.library.Architectures.layeredArchitecture;

import com.tngtech.archunit.junit.AnalyzeClasses;
import com.tngtech.archunit.junit.ArchTest;
import com.tngtech.archunit.lang.ArchRule;

@AnalyzeClasses(packages = "io.mcn.settlement")
class ArchitectureTest {
  @ArchTest
  static final ArchRule hexagonal =
      layeredArchitecture()
          .consideringOnlyDependenciesInLayers()
          .withOptionalLayers(true)
          .layer("Domain")
          .definedBy("..domain..")
          .layer("Application")
          .definedBy("..application..")
          .layer("Adapter")
          .definedBy("..adapter..")
          .whereLayer("Adapter")
          .mayNotBeAccessedByAnyLayer()
          .whereLayer("Application")
          .mayOnlyBeAccessedByLayers("Adapter")
          .whereLayer("Domain")
          .mayOnlyBeAccessedByLayers("Application", "Adapter");
}
