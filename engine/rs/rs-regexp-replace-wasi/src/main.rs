use std::io;

use io::Read;
use io::Write;

use regex::Regex;

#[derive(serde::Serialize)]
struct ErrorDto {
    code: i32,
    message: String,
}

#[derive(serde::Serialize)]
struct ReplaceResult {
    replaced_text: String,
    error: Option<ErrorDto>,
}

#[derive(serde::Deserialize)]
struct UntrustedInput {
    pattern: String,
    text: String,
    replacement: String,
}

impl UntrustedInput {
    fn to_result(&self) -> Result<String, ReplexErr> {
        let pat = Regex::new(self.pattern.as_str())
            .map_err(|e| ReplexErr::InvalidPattern(format!("invalid regular expression: {e}")))?;

        let replaced = pat.replace_all(self.text.as_str(), self.replacement.as_str());
        Ok(replaced.into_owned())
    }
}

enum ReplexErr {
    InvalidInput(String),
    InvalidPattern(String),
    IoError(io::Error),
}

const ERR_CODE_INVALID_INPUT: i32 = 1;
const ERR_CODE_INVALID_PATTERN: i32 = 2;

fn sub() -> Result<String, ReplexErr> {
    let ijson_max: u64 = 1048576;
    let iraw = io::stdin();
    let i = iraw.lock();
    let mut taken = i.take(ijson_max);
    let mut ijson: Vec<u8> = vec![];
    taken.read_to_end(&mut ijson).map_err(ReplexErr::IoError)?;

    let parsed: UntrustedInput = serde_json::from_slice(&ijson)
        .map_err(|e| ReplexErr::InvalidInput(format!("unable to parse the input json: {e}")))?;

    parsed.to_result()
}

fn main() -> Result<(), io::Error> {
    let replaced: Result<String, ReplexErr> = sub();

    let rslt: ReplaceResult = match replaced {
        Ok(s) => ReplaceResult {
            replaced_text: s,
            error: None,
        },
        Err(ReplexErr::IoError(i)) => return Err(i),
        Err(ReplexErr::InvalidInput(i)) => ReplaceResult {
            replaced_text: "".into(),
            error: Some(ErrorDto {
                code: ERR_CODE_INVALID_INPUT,
                message: i,
            }),
        },
        Err(ReplexErr::InvalidPattern(i)) => ReplaceResult {
            replaced_text: "".into(),
            error: Some(ErrorDto {
                code: ERR_CODE_INVALID_PATTERN,
                message: i,
            }),
        },
    };

    let o = io::stdout();
    let mut ol = o.lock();

    serde_json::to_writer(&mut ol, &rslt)?;

    ol.flush()?;
    Ok(())
}