"""Recovery manager for orchestrating rollbacks."""

import logging
from typing import Any, Callable, Dict, List, Optional
from dataclasses import dataclass

from .registry import CompensationRegistry

logger = logging.getLogger(__name__)


@dataclass
class RollbackStep:
    """A single step in a rollback plan."""
    transaction_id: str
    tool_name: str
    original_input: Dict
    original_output: Dict
    action: str  # 'compensate' or 'skip'
    compensator_name: Optional[str] = None
    compensation_params: Optional[Dict] = None
    reason: Optional[str] = None


@dataclass
class RollbackResult:
    """Result of a rollback execution."""
    total: int
    compensated: int
    skipped: int
    failed: int
    errors: List[str]


class RecoveryManager:
    """
    Manages recovery and rollback operations for agent sessions.

    The RecoveryManager uses the transaction log and compensation registry
    to generate and execute rollback plans when failures occur.
    """

    def __init__(
        self,
        registry: CompensationRegistry,
        tool_executor: Optional[Callable[[str, Dict], Any]] = None,
    ):
        """
        Initialize the recovery manager.

        Args:
            registry: CompensationRegistry with approved mappings
            tool_executor: Optional function to execute tools by name
        """
        self.registry = registry
        self.tool_executor = tool_executor
        self._transactions: List[Dict] = []

    def record_transaction(
        self,
        transaction_id: str,
        tool_name: str,
        input_params: Dict,
        output_result: Dict,
    ) -> None:
        """Record a transaction for potential rollback."""
        self._transactions.append({
            "id": transaction_id,
            "tool_name": tool_name,
            "input": input_params,
            "output": output_result,
        })

    def generate_rollback_plan(self) -> List[RollbackStep]:
        """
        Generate a rollback plan for all recorded transactions.

        Returns:
            List of RollbackStep objects in reverse order (LIFO)
        """
        # Ensure registry is up to date
        self.registry.sync()

        steps = []
        # Process transactions in reverse order
        for tx in reversed(self._transactions):
            tool_name = tx["tool_name"]
            mapping = self.registry.get_compensator(tool_name)

            if not mapping:
                steps.append(RollbackStep(
                    transaction_id=tx["id"],
                    tool_name=tool_name,
                    original_input=tx["input"],
                    original_output=tx["output"],
                    action="skip",
                    reason="No approved compensation mapping",
                ))
            else:
                # Build compensation parameters
                params = {}
                param_mapping = mapping.get("parameter_mapping", {})
                for param_name, source in param_mapping.items():
                    value = self.registry._extract_value(
                        source, tx["input"], tx["output"]
                    )
                    if value is not None:
                        params[param_name] = value

                steps.append(RollbackStep(
                    transaction_id=tx["id"],
                    tool_name=tool_name,
                    original_input=tx["input"],
                    original_output=tx["output"],
                    action="compensate",
                    compensator_name=mapping.get("compensator"),
                    compensation_params=params,
                ))

        return steps

    def execute_rollback(
        self,
        plan: Optional[List[RollbackStep]] = None,
    ) -> RollbackResult:
        """
        Execute a rollback plan.

        Args:
            plan: Optional pre-generated plan. If None, generates a new one.

        Returns:
            RollbackResult with execution statistics
        """
        if plan is None:
            plan = self.generate_rollback_plan()

        result = RollbackResult(
            total=len(plan),
            compensated=0,
            skipped=0,
            failed=0,
            errors=[],
        )

        for step in plan:
            if step.action == "skip":
                result.skipped += 1
                logger.info(f"Skipping {step.tool_name}: {step.reason}")
            elif step.action == "compensate":
                try:
                    if self.tool_executor:
                        self.tool_executor(
                            step.compensator_name,
                            step.compensation_params or {},
                        )
                    else:
                        # Try using the registry's local functions
                        self.registry.execute_compensation(
                            step.tool_name,
                            step.original_input,
                            step.original_output,
                        )
                    result.compensated += 1
                    logger.info(
                        f"Compensated {step.tool_name} with {step.compensator_name}"
                    )
                except Exception as e:
                    result.failed += 1
                    error_msg = f"Failed to compensate {step.tool_name}: {e}"
                    result.errors.append(error_msg)
                    logger.error(error_msg)

        # Clear transactions after rollback
        self._transactions.clear()

        return result

    def clear_transactions(self) -> None:
        """Clear all recorded transactions."""
        self._transactions.clear()

    @property
    def transaction_count(self) -> int:
        """Get the number of recorded transactions."""
        return len(self._transactions)
