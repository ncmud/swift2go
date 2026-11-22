public func substringContainsString() {
    let rendered = "<pre>\">  <span>a &lt; b</span>   c<br></pre>"
    let shown = rendered.dropFirst(7)
    print(shown.contains("a &lt; b"), shown.contains("   c"), shown.contains("zz"), shown.contains("c"), shown.contains(Character("q")))
}
