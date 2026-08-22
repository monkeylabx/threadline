import SwiftUI

struct ThreadlineMessageComposer: View {
  @Environment(\.accessibilityReduceMotion) private var reduceMotion

  var body: some View {
    Text("Message")
      .font(.body)
      .padding(ThreadlineTokens.Space.md)
      .animation(
        reduceMotion ? nil : .easeOut(duration: Double(ThreadlineTokens.MotionDurationMs.standard) / 1000),
        value: reduceMotion
      )
  }
}
