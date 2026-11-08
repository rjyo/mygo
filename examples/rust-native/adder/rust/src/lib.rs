//! The functions the Go package adder binds with package ffi. They take and
//! return what C does: integers, floats, pointers and NUL-terminated strings.

use std::ffi::{c_char, CStr, CString};

#[no_mangle]
pub extern "C" fn add(a: i32, b: i32) -> i32 {
    a.wrapping_add(b)
}

#[no_mangle]
pub extern "C" fn fibonacci(n: u32) -> u64 {
    let (mut a, mut b) = (0u64, 1u64);
    for _ in 0..n {
        (a, b) = (b, a.wrapping_add(b));
    }
    a
}

/// Counts the vowels of a string Go passes, which lives for the call only.
#[no_mangle]
pub extern "C" fn count_vowels(s: *const c_char) -> u32 {
    if s.is_null() {
        return 0;
    }
    let s = unsafe { CStr::from_ptr(s) }.to_string_lossy();
    s.chars().filter(|c| "aeiouAEIOU".contains(*c)).count() as u32
}

/// Returns a static string: Go copies it, and nothing needs freeing.
#[no_mangle]
pub extern "C" fn adder_version() -> *const c_char {
    concat!("adder ", env!("CARGO_PKG_VERSION"), "\0").as_ptr() as *const c_char
}

/// Returns a string Rust allocates, which Go frees with free_string.
#[no_mangle]
pub extern "C" fn greeting(name: *const c_char) -> *mut c_char {
    let name = if name.is_null() {
        "".into()
    } else {
        unsafe { CStr::from_ptr(name) }.to_string_lossy()
    };
    CString::new(format!("Hello, {name}!")).unwrap_or_default().into_raw()
}

/// Frees a string greeting returned.
#[no_mangle]
pub extern "C" fn free_string(s: *mut c_char) {
    if !s.is_null() {
        drop(unsafe { CString::from_raw(s) });
    }
}
