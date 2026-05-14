pub trait SecretFilter {
    fn redact(&self, input: &str) -> String;
}

#[derive(Clone, Copy, Debug, Default)]
pub struct NoopSecretFilter;

impl SecretFilter for NoopSecretFilter {
    fn redact(&self, input: &str) -> String {
        input.to_owned()
    }
}
