from __future__ import annotations
from collections import namedtuple
from enum import Enum
import json
import pkgutil
from typing import TYPE_CHECKING, List
import typing
from BaseClasses import Location
from .names import RegionNames


if TYPE_CHECKING:
    from .world import PlanetZooWorld


class LocationFormation:
    def __init__(self, stringid, label, type, species_type, water_needed, fence_grade, id):
        self.stringid = stringid
        self.label = label
        self.type = type
        self.species_type = species_type
        self.water_needed = water_needed
        self.fence_grade = fence_grade
        self.id = id
    # stringid: str
    # label: str
    # type: LocationType
    # species_type: str
    # water_needed: bool
    # fence_grade: int


def location_object_decoder(l_obj):
    return LocationFormation(l_obj['stringid'], l_obj['label'], l_obj['type'], l_obj['species_type'], l_obj['water_needed'], l_obj['fence_grade'], l_obj['id'])


# def location_decoder(l_objdict):
#     return namedtuple('LocationFormation', l_objdict.keys())(*l_objdict.values())


class LocationType(str, Enum):
    research_welfare = "research welfare"
    firsts = "firsts"
    conservation = "conservation"
    mechanic = "mechanic"
    milestones = "milestones"


class LocationData(typing.NamedTuple):
    LocationName: str


location_raw_data = pkgutil.get_data(
    __name__, "data/locations.json")
if location_raw_data:
    location_data = json.loads(
        location_raw_data, object_hook=location_object_decoder)

complete_location_list: List[LocationFormation] = list(location_data)


class PlanetZooLocation(Location):
    game = "Planet Zoo"


research_locations_names = [
    location
    for location in complete_location_list
    if location.type == LocationType.research_welfare
]

mechanic_locations_names = [
    location
    for location in complete_location_list
    if location.type == LocationType.mechanic
]

firsts_locations_names = [
    location
    for location in complete_location_list
    if location.type == LocationType.firsts
]

milestones_locations_names = [
    location
    for location in complete_location_list
    if location.type == LocationType.milestones
]

conservation_locations_names = [
    location
    for location in complete_location_list
    if location.type == LocationType.conservation
]

all_locations = [
    research_locations_names,
    mechanic_locations_names,
    firsts_locations_names,
    milestones_locations_names,
    conservation_locations_names]


def get_location_names_with_ids(location_names: list[LocationFormation]) -> dict[str, int]:
    result: dict[str, int] = {}
    for location_name in location_names:
        assert location_name.id is not None, f"{location_name.label} has no id assigned"
        result[location_name.label] = location_name.id
    return result


def create_all_locations(world: PlanetZooWorld) -> None:
    create_regular_locations(world)
    create_goal_location(world)


def create_regular_locations(world: PlanetZooWorld) -> None:
    research_locations = world.multiworld.get_region(
        RegionNames.research, world.player)
    mechanic_locations = world.multiworld.get_region(
        RegionNames.mechanic, world.player)
    conservation_locations = world.multiworld.get_region(
        RegionNames.conservation, world.player)
    milestones_locations = world.multiworld.get_region(
        RegionNames.milestones, world.player)
    firsts_locations = world.multiworld.get_region(
        RegionNames.firsts, world.player)

    set_research_locations = get_location_names_with_ids(
        research_locations_names)
    research_locations.add_locations(set_research_locations, PlanetZooLocation)

    set_mechanic_locations = get_location_names_with_ids(
        mechanic_locations_names)
    mechanic_locations.add_locations(set_mechanic_locations, PlanetZooLocation)

    set_conservation_locations = get_location_names_with_ids(
        conservation_locations_names)
    conservation_locations.add_locations(
        set_conservation_locations, PlanetZooLocation)

    set_milestones_locations = get_location_names_with_ids(
        milestones_locations_names)
    milestones_locations.add_locations(
        set_milestones_locations, PlanetZooLocation)

    # Remove fb_giant_panda out of the normal location pool

    filtered_firsts_locations = [
        loc for loc in firsts_locations_names if loc.label != "Giant Panda - First Breeding"]
    set_firsts_locations = get_location_names_with_ids(
        filtered_firsts_locations)
    firsts_locations.add_locations(set_firsts_locations, PlanetZooLocation)

# Current goal option : Breed Pandas


def create_goal_location(world: PlanetZooWorld) -> None:
    region = world.multiworld.get_region(RegionNames.menu, world.player)
    panda_location = next(
        loc for loc in complete_location_list if loc.stringid == "fb_gpanda")
    goal_location = PlanetZooLocation(
        world.player, panda_location.label, None, region)
    region.locations.append(goal_location)
