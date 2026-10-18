import SwiftUI

/// AppItemIcon is ItemIcon with the website icon a login shows, which the app model loads once per domain.
struct AppItemIcon: View {
    var item: ItemDetail
    var size: CGFloat = 32

    @EnvironmentObject private var model: AppModel

    var body: some View {
        let websiteURL = item.websiteIconURL
        ItemIcon(item: item, size: size, websiteIcon: websiteURL.flatMap { model.getWebsiteIcon(url: $0) })
            .task(id: websiteURL) {
                if let websiteURL {
                    model.loadWebsiteIcon(url: websiteURL)
                }
            }
    }
}
