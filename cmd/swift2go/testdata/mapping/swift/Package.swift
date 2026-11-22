// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Mapping",
    targets: [
        .target(name: "Glue"),
        .target(name: "Toy", dependencies: ["Glue"]),
        .target(name: "App", dependencies: ["Toy", "Glue"]),
    ]
)
