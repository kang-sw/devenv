#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct HarnessCapabilities {
    pub supports_mcp: bool,
    pub supports_skills: bool,
    pub supports_api_models: bool,
    pub supports_secret_filtering: bool,
}

pub trait HarnessProvider {
    fn capabilities(&self) -> HarnessCapabilities;
}
