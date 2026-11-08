//! The implementation of package roundtrip in Rust: every value it is
//! given, back, as its Swift and Zig implementations do.

mod mygo;

use mygo::*;
use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicI64, Ordering};
use std::sync::Arc;
use std::time::{Duration, SystemTime};

pub fn make_echo() -> Result<Arc<dyn Echo>, Error> {
    Ok(Arc::new(Echoer))
}

struct Echoer;

impl Echo for Echoer {
    fn scalars(&self, i: i64, i8: i8, u16: u16, i32: i32, u64: u64, f32: f32, f: f64, b: bool) -> Scalars {
        Scalars {
            i: i + 1,
            i8: i8.wrapping_add(1),
            u16: u16.wrapping_add(1),
            i32: i32 + 1,
            u64: u64.wrapping_add(1),
            f32: f32 + 1.0,
            f: f + 1.0,
            b: !b,
        }
    }

    fn add(&self, a: i64, b: i64) -> i64 {
        a + b
    }

    fn text(&self, s: String, b: Vec<u8>) -> Texts {
        Texts {
            s: s.repeat(2),
            b: b.into_iter().rev().collect(),
        }
    }

    fn record(&self, mut r: Record) -> Result<Record, Error> {
        for c in r.counts.iter_mut() {
            *c *= 2;
        }
        Ok(r)
    }

    fn lists(&self, words: Vec<String>, nested: Vec<Vec<i64>>) -> Result<HashMap<String, i64>, Error> {
        let mut m: HashMap<String, i64> = words.iter().map(|w| (w.clone(), w.len() as i64)).collect();
        m.insert("nested".into(), nested.iter().flatten().sum());
        Ok(m)
    }

    fn maybe(&self, p: Option<i64>) -> Option<i64> {
        p.map(|v| v + 1)
    }

    fn times(&self, t: SystemTime, d: Duration) -> Result<Moment, Error> {
        Ok(Moment { at: t + d, twice: d * 2 })
    }

    fn kind(&self, k: Kind, l: Level) -> Result<Kinds, Error> {
        let next = match k {
            Kind::KindA => Kind::KindB,
            Kind::KindB => Kind::KindC,
            Kind::KindC => Kind::KindA,
        };
        let level = if l == Level::Low { Level::High } else { Level::Low };
        Ok(Kinds { kind: next, level })
    }

    fn fail(&self, name: String) -> Result<(), Error> {
        match name.as_str() {
            "" => Ok(()),
            "named" => Err(RoundtripError::Named.into()),
            "unsupported" => Err(Error::Unsupported),
            _ => Err(Error::Other {
                domain: "RoundtripDomain".into(),
                number: 7,
                message: name,
            }),
        }
    }

    fn wait(&self, ctx: &Context, d: Duration, s: String) -> Result<String, Error> {
        ctx.sleep(d)?;
        Ok(s)
    }

    fn count(&self, _ctx: &Context, n: i64, fail: i64) -> Stream<Result<i64, Error>> {
        let mut i = 0;
        stream_of(std::iter::from_fn(move || {
            if i >= n || i > fail && fail >= 0 {
                return None;
            }
            i += 1;
            if i - 1 == fail {
                return Some(Err(RoundtripError::Named.into()));
            }
            Some(Ok(i - 1))
        }))
    }

    fn records(&self, _ctx: &Context, n: i64) -> Stream<Result<Record, Error>> {
        stream_of((0..n).map(|i| {
            Ok(Record {
                name: format!("r{}", i),
                counts: vec![i],
                tags: HashMap::from([("i".to_string(), i.to_string())]),
                inner: Inner { x: i as f64, y: 0.0 },
                next: None,
                kind: Kind::KindA,
                data: vec![(i & 0xff) as u8],
            })
        }))
    }

    fn counter(&self, n: i64) -> Result<Arc<dyn Counter>, Error> {
        Ok(Arc::new(Counting(AtomicI64::new(n))))
    }

    fn each(&self, words: Vec<String>, f: Arc<dyn Fn(String, i64) + Send + Sync>) -> i64 {
        for (i, w) in words.iter().enumerate() {
            f(w.clone(), i as i64);
        }
        words.len() as i64
    }

    fn ask(&self, ns: Vec<i64>, f: Arc<dyn Fn(i64) -> Result<i64, Error> + Send + Sync>) -> Result<i64, Error> {
        let mut sum = 0;
        for n in ns {
            sum += f(n)?;
        }
        Ok(sum)
    }

    fn ticks(&self, interval: Duration, f: Arc<dyn Fn(i64) + Send + Sync>) -> Box<dyn Closer> {
        let stop = Arc::new(AtomicBool::new(false));
        let s = Arc::clone(&stop);
        std::thread::spawn(move || {
            let mut n = 0;
            while !s.load(Ordering::SeqCst) {
                std::thread::sleep(interval);
                if s.load(Ordering::SeqCst) {
                    break;
                }
                n += 1;
                f(n);
            }
        });
        closer(move || stop.store(true, Ordering::SeqCst))
    }

    fn r#use(&self, ctx: &Context, s: Arc<dyn Scorer>, words: Vec<String>) -> Result<f64, Error> {
        let mut sum: f64 = words.iter().map(|w| s.score(w.clone())).sum();
        if let Some(first) = words.first() {
            let why = s.explain(ctx, first.clone())?;
            sum += why.len() as f64;
        }
        Ok(sum)
    }

    fn fill(&self, mut buf: Buffer, x: i64) -> Result<i64, Error> {
        for (i, b) in buf.bytes_mut().iter_mut().enumerate() {
            *b = (i as i64 ^ x) as u8;
        }
        Ok(buf.len() as i64)
    }

    fn mix(&self, f: f32, d: f64, b: bool, i: i8) -> f64 {
        f as f64 + d + if b { 1.0 } else { 0.0 } + i as f64
    }

    fn r#box(&self, r: Rect) -> Result<Rect, Error> {
        Ok(Rect {
            x: r.x + r.w,
            y: r.y + r.h,
            ..r
        })
    }

    fn nothing(&self, e: Vec<Empty>) -> i64 {
        e.len() as i64
    }

    fn on_main(&self, r: Record, f: Arc<dyn Fn(Inner) + Send + Sync>) -> Result<Texts, Error> {
        f(r.inner.clone());
        Ok(Texts { s: r.name, b: r.data })
    }
}

struct Counting(AtomicI64);

impl Counter for Counting {
    fn close(&self) -> Result<(), Error> {
        Ok(())
    }

    fn next(&self) -> i64 {
        self.0.fetch_add(1, Ordering::SeqCst)
    }

    fn same(&self) -> Arc<dyn Counter> {
        Arc::new(Counting(AtomicI64::new(self.0.load(Ordering::SeqCst))))
    }
}
