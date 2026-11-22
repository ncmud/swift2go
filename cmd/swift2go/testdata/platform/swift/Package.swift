// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "platform",
    targets: [
        .target(name: "Platform", resources: [.copy("motd.txt")]),
    ]
)
