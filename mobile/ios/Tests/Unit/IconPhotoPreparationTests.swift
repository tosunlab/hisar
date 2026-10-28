import XCTest
@testable import Hisar

@MainActor
final class IconPhotoPreparationTests: XCTestCase {
    private final class Gate<Value> {
        var continuation: CheckedContinuation<Value, Error>?
        func wait() async throws -> Value {
            try await withCheckedThrowingContinuation { continuation = $0 }
        }
        func ready() async { while continuation == nil { await Task.yield() } }
        func finish(_ result: Result<Value, Error>) { continuation?.resume(with: result); continuation = nil }
    }

    func testIconPhotoPreparationRejectsStaleResults() async {
        let preparation = IconPhotoPreparation()
        preparation.activate()
        var icon: ChosenIcon? = ChosenIcon(preset: "code")
        for selection in [ChosenIcon(preset: "bank"), nil] {
            let transfer = Gate<Data?>()
            var converted = false
            let task = preparation.prepare(load: { try await transfer.wait() }, convert: { data in converted = true; return data }, apply: { icon = $0 })
            await transfer.ready()
            XCTAssertTrue(preparation.isPreparing, "Done must be disabled until preparation completes")
            preparation.cancel()
            icon = selection
            transfer.finish(.success(Data([1])))
            await task.value
            XCTAssertEqual(icon, selection, "an older photo cannot replace a preset or Automatic")
            XCTAssertFalse(converted, "a stale transfer never starts conversion")
            XCTAssertFalse(preparation.isPreparing)
        }

        let oldConversion = Gate<Data>()
        let old = preparation.prepare(load: { Data([1]) }, convert: { _ in try await oldConversion.wait() }, apply: { icon = $0 })
        await oldConversion.ready()
        let newTransfer = Gate<Data?>()
        let newer = preparation.prepare(load: { try await newTransfer.wait() }, convert: { $0 }, apply: { icon = $0 })
        await newTransfer.ready()
        oldConversion.finish(.failure(BackendError(message: "stale error")))
        await old.value
        XCTAssertTrue(preparation.isPreparing, "stale completion cannot clear a newer photo's pending state")
        XCTAssertNil(preparation.errorMessage)
        newTransfer.finish(.success(Data([2])))
        await newer.value
        XCTAssertEqual(icon, ChosenIcon(image: Data([2])))
        XCTAssertFalse(preparation.isPreparing)

        for dismiss in [false, true] {
            let conversion = Gate<Data>()
            let pending = preparation.prepare(load: { Data([3]) }, convert: { _ in try await conversion.wait() }, apply: { icon = $0 })
            await conversion.ready()
            if dismiss { preparation.dismiss() } else { preparation.cancel() }
            conversion.finish(.success(Data([3])))
            await pending.value
            XCTAssertEqual(icon, ChosenIcon(image: Data([2])), "Cancel preparation and swipe dismissal preserve the last completed icon")
            XCTAssertFalse(preparation.isPreparing)
        }
        preparation.activate()
        let failure = preparation.prepare(load: { throw BackendError(message: "Photo refused") }, convert: { $0 }, apply: { icon = $0 })
        await failure.value
        XCTAssertEqual(preparation.errorMessage, "Photo refused")
        XCTAssertEqual(icon, ChosenIcon(image: Data([2])))
        XCTAssertFalse(preparation.isPreparing)
    }
}
