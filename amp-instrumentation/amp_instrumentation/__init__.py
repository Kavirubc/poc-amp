"""AMP Instrumentation - Tool interception and compensation for AI agents."""

from .interceptor import AMPInterceptor, tool_wrapper
from .registry import CompensationRegistry
from .recovery import RecoveryManager

__version__ = "0.1.0"
__all__ = ["AMPInterceptor", "tool_wrapper", "CompensationRegistry", "RecoveryManager"]
