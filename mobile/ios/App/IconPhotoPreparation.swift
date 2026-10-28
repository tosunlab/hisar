import Foundation
import Combine

@MainActor
final class IconPhotoPreparation: ObservableObject {
    @Published private(set) var isPreparing = false
    @Published private(set) var errorMessage: String?
    private var task: Task<Void, Never>?
    private var generation = 0
    private var isActive = false

    func activate() { isActive = true }

    func cancel() {
        generation += 1
        task?.cancel()
        task = nil
        isPreparing = false
        errorMessage = nil
    }

    func dismiss() {
        isActive = false
        cancel()
    }

    @discardableResult
    func prepare(
        load: @escaping () async throws -> Data?,
        convert: @escaping (Data) async throws -> Data,
        apply: @escaping (ChosenIcon) -> Void
    ) -> Task<Void, Never> {
        cancel()
        let selection = generation
        isPreparing = true
        let pending = Task { [weak self] in
            guard let self else { return }
            defer {
                if self.generation == selection {
                    self.isPreparing = false
                    self.task = nil
                }
            }
            do {
                let data = try await load()
                guard self.isActive, self.generation == selection, !Task.isCancelled else { return }
                guard let data else { throw BackendError(message: AppModel.unreadablePhotoMessage) }
                let image = try await convert(data)
                guard self.isActive, self.generation == selection, !Task.isCancelled else { return }
                apply(ChosenIcon(image: image))
            } catch {
                guard self.isActive, self.generation == selection, !Task.isCancelled else { return }
                self.errorMessage = describeError(error)
            }
        }
        task = pending
        return pending
    }
}
