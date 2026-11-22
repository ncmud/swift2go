// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Traits",
    targets: [
        .target(name: "Lib"),
        .target(name: "Scopes", dependencies: ["Lib"]),
        .testTarget(name: "LibTests", dependencies: ["Lib", "Scopes"]),
    ]
)
