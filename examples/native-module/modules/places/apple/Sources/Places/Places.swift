import AppKit
import Language
import MapKit

// The implementation of package places on macOS: a map of MapKit, and the
// share menu of AppKit.

@MainActor func makeMap() throws -> any Map { MapView() }

@MainActor final class MapView: MKMapView, Map, MKMapViewDelegate {
    private var moved: [UUID: @Sendable (Place) -> Void] = [:]

    override init(frame: NSRect) {
        super.init(frame: frame)
        delegate = self
    }

    required init?(coder: NSCoder) { fatalError("not from a nib") }

    func show(p: Place, span: Double) {
        let center = CLLocationCoordinate2D(latitude: p.latitude, longitude: p.longitude)
        setRegion(MKCoordinateRegion(center: center, span: MKCoordinateSpan(latitudeDelta: span, longitudeDelta: span)), animated: false)
    }

    func center() -> Place {
        Place(latitude: centerCoordinate.latitude, longitude: centerCoordinate.longitude)
    }

    func pin(p: Place, a: any Language.Analyzer, text: String) {
        let pin = MKPointAnnotation()
        pin.coordinate = CLLocationCoordinate2D(latitude: p.latitude, longitude: p.longitude)
        pin.title = "\((try? a.detectLanguage(text: text)) ?? "?"): \(text)"
        addAnnotation(pin)
    }

    func onMove(f: @escaping @Sendable (Place) -> Void) -> any Closer {
        let id = UUID()
        moved[id] = f
        return NativeCloser { [weak self] in
            MainActor.assumeIsolated { _ = self?.moved.removeValue(forKey: id) }
        }
    }

    nonisolated func mapView(_ mapView: MKMapView, regionDidChangeAnimated animated: Bool) {
        MainActor.assumeIsolated {
            let c = center()
            for f in moved.values { f(c) }
        }
    }
}

@MainActor func makeSharing() throws -> any Sharing { SystemSharing() }

@MainActor final class SystemSharing: Sharing {
    func share(win: NSWindow, rect: CGRect, text: String) {
        guard let view = win.contentView else { return }
        NSSharingServicePicker(items: [text]).show(relativeTo: rect, of: view, preferredEdge: .minY)
    }

    func shareFrom(view: NSView, text: String) {
        NSSharingServicePicker(items: [text]).show(relativeTo: view.bounds, of: view, preferredEdge: .minY)
    }
}
