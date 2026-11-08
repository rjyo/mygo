import CoreGraphics
import CoreVideo
import Foundation

// The implementation of package motion on macOS: hues turning, drawn with
// Core Graphics into pixel buffers that native UI shows.

func makeAnimation() throws -> any Animation { HueAnimation() }

final class HueAnimation: Animation {
    func attach(tex: NativeTexture, size: Int) -> any Closer {
        var attrs = tex.pixelBufferAttributes
        attrs[kCVPixelBufferWidthKey as String] = size
        attrs[kCVPixelBufferHeightKey as String] = size
        var pool: CVPixelBufferPool?
        CVPixelBufferPoolCreate(nil, nil, attrs as CFDictionary, &pool)
        guard let pool else { return NativeCloser {} }
        let start = CACurrentMediaTimeCompat()
        let link = tex.displayLink { time in
            var buffer: CVPixelBuffer?
            guard CVPixelBufferPoolCreatePixelBuffer(nil, pool, &buffer) == kCVReturnSuccess, let buffer else { return }
            draw(into: buffer, size: size, t: time - start)
            tex.present(buffer)
        }
        return NativeCloser { link.invalidate() }
    }
}

/// The host time in seconds, as the display link counts.
func CACurrentMediaTimeCompat() -> CFTimeInterval {
    Double(CVGetCurrentHostTime()) / CVGetHostClockFrequency()
}

/// Draws a disc of hues, turned by t seconds, into buffer.
func draw(into buffer: CVPixelBuffer, size: Int, t: Double) {
    CVPixelBufferLockBaseAddress(buffer, [])
    defer { CVPixelBufferUnlockBaseAddress(buffer, []) }
    guard let ctx = CGContext(
        data: CVPixelBufferGetBaseAddress(buffer), width: size, height: size, bitsPerComponent: 8,
        bytesPerRow: CVPixelBufferGetBytesPerRow(buffer), space: CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue
    ) else { return }
    let s = CGFloat(size)
    ctx.clear(CGRect(x: 0, y: 0, width: s, height: s))
    let slices = 36
    for i in 0..<slices {
        let a0 = Double(i) / Double(slices) * 2 * .pi + t
        let a1 = Double(i + 1) / Double(slices) * 2 * .pi + t
        let hue = Double(i) / Double(slices) * 2 * .pi // the slice's own, which turns with it
        ctx.setFillColor(CGColor(red: 0.5 + 0.5 * cos(hue), green: 0.5 + 0.5 * cos(hue + 2.094), blue: 0.5 + 0.5 * cos(hue + 4.189), alpha: 1))
        ctx.move(to: CGPoint(x: s / 2, y: s / 2))
        ctx.addArc(center: CGPoint(x: s / 2, y: s / 2), radius: s / 2, startAngle: a0, endAngle: a1 + 0.01, clockwise: false)
        ctx.fillPath()
    }
}
