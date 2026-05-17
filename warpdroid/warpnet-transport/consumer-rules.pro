# gomobile ships a native Go runtime with reflection-heavy entry points.
# Keep the generated node package and its seq bridge classes so R8 does not
# strip them from release builds.
-keep class node.** { *; }
-keep class go.** { *; }
