plugins {
  base
}

tasks.register("workspaceBuild") {
  group = "build"
  description = "Builds the dependency-free Kotlin skeleton."
  dependsOn(":apps:android:buildKotlinSkeleton")
}

tasks.register("workspaceTest") {
  group = "verification"
  description = "Runs the dependency-free Kotlin skeleton test."
  dependsOn(":apps:android:testKotlinSkeleton")
}
