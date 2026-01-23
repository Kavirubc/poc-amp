"""Compensation registry for managing tool-to-compensator mappings."""

import logging
from typing import Any, Callable, Dict, Optional

import requests

logger = logging.getLogger(__name__)


class CompensationRegistry:
    """
    Local registry of compensation mappings that syncs with AMP.

    This class maintains a local cache of approved compensation mappings
    and provides methods to look up compensators for tools.
    """

    def __init__(
        self,
        agent_id: str,
        amp_url: str = "http://localhost:8080",
    ):
        self.agent_id = agent_id
        self.amp_url = amp_url.rstrip("/")
        self._mappings: Dict[str, Dict] = {}
        self._tool_functions: Dict[str, Callable] = {}

    def sync(self) -> bool:
        """
        Sync the local registry with AMP's approved mappings.

        Returns:
            True if sync was successful, False otherwise
        """
        try:
            response = requests.get(
                f"{self.amp_url}/api/v1/agents/{self.agent_id}/compensation-mappings/approved",
                timeout=10,
            )
            response.raise_for_status()
            result = response.json()
            self._mappings = result.get("registry", {})
            logger.info(f"Synced {len(self._mappings)} compensation mappings")
            return True
        except Exception as e:
            logger.error(f"Failed to sync compensation registry: {e}")
            return False

    def register_compensator(self, tool_name: str, compensator_func: Callable) -> None:
        """
        Register a local compensator function for a tool.

        Args:
            tool_name: The tool that this compensator undoes
            compensator_func: The function to call for compensation
        """
        self._tool_functions[tool_name] = compensator_func

    def get_compensator(self, tool_name: str) -> Optional[Dict]:
        """
        Get the compensation info for a tool.

        Args:
            tool_name: Name of the tool

        Returns:
            Dict with compensator name and parameter mapping, or None
        """
        return self._mappings.get(tool_name)

    def has_compensator(self, tool_name: str) -> bool:
        """Check if a tool has an approved compensator."""
        return tool_name in self._mappings

    def execute_compensation(
        self,
        tool_name: str,
        original_input: Dict,
        original_output: Dict,
    ) -> Optional[Any]:
        """
        Execute the compensation for a tool.

        Args:
            tool_name: The tool to compensate
            original_input: The original input parameters
            original_output: The original output result

        Returns:
            The result of the compensation call, or None if no compensator
        """
        mapping = self.get_compensator(tool_name)
        if not mapping:
            logger.warning(f"No compensator found for {tool_name}")
            return None

        compensator_name = mapping.get("compensator")
        param_mapping = mapping.get("parameter_mapping", {})

        # Build compensation parameters
        params = {}
        for param_name, source in param_mapping.items():
            value = self._extract_value(source, original_input, original_output)
            if value is not None:
                params[param_name] = value

        # Execute the compensator if registered locally
        if compensator_name in self._tool_functions:
            try:
                result = self._tool_functions[compensator_name](**params)
                logger.info(f"Executed compensation {compensator_name} for {tool_name}")
                return result
            except Exception as e:
                logger.error(f"Compensation {compensator_name} failed: {e}")
                raise

        logger.warning(f"Compensator {compensator_name} not registered locally")
        return None

    def _extract_value(
        self,
        source: str,
        input_params: Dict,
        output_result: Dict,
    ) -> Optional[Any]:
        """Extract a value from input or output based on source path."""
        parts = source.split(".")
        if len(parts) < 2:
            return None

        if parts[0] == "input":
            data = input_params
        elif parts[0] in ("result", "output"):
            data = output_result
        else:
            return None

        # Navigate the path
        current = data
        for key in parts[1:]:
            if isinstance(current, dict) and key in current:
                current = current[key]
            else:
                return None

        return current

    @property
    def mappings(self) -> Dict[str, Dict]:
        """Get all current mappings."""
        return self._mappings.copy()
