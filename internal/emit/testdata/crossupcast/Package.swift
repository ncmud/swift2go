// swift-tools-version:6.1
import PackageDescription

let package = Package(
    name: "CrossUpcast",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "Base"),
        .target(name: "Kit", dependencies: ["Base"]),
        .target(name: "App", dependencies: ["Base", "Kit"]),
    ]
)
