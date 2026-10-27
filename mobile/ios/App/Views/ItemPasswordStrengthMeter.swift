import SwiftUI

struct ItemPasswordStrengthMeter: View {
    var password: String
    @EnvironmentObject private var model: AppModel
    @State private var strength: PasswordStrength?

    var body: some View {
        Group {
            if !password.isEmpty, let strength {
                PasswordStrengthMeter(strength: strength)
            }
        }
        .task(id: password) {
            strength = nil
            guard !password.isEmpty else { return }
            let result = await model.ratePassword(password)
            guard !Task.isCancelled else { return }
            strength = result
        }
    }
}
