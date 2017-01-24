import pytest
import cattle
import requests
from wait_for import wait_for


@pytest.fixture
def client():
    url = 'http://localhost:8088/v1-catalog/schemas'
    templates = cattle.from_env(url=url).list_template(catalogId='qa-catalog')
    wait_for(
        lambda: len(templates) > 0
    )
    return cattle.from_env(url=url)


def test_catalog_list(client):
    catalogs = client.list_catalog()
    assert len(catalogs) > 0


def test_catalog_state_uri_present(client):
    catalogs = client.list_catalog()
    assert len(catalogs) > 0

    for i in range(len(catalogs)):
        assert catalogs[i].state is not None
        assert catalogs[i].uri is not None
        assert catalogs[i].branch is not None


def test_template_list(client):
    templates = client.list_template()
    assert len(templates) > 0


def test_refresh_catalog(client):
    url = 'http://localhost:8088/v1-catalog/templates?action=refresh'
    response = requests.post(url)
    assert response.status_code == 204


def test_catalog_not_found(client):
    url = 'http://localhost:8088/v1-catalog/catalogs/abc'
    response = requests.get(url)
    assert response.status_code == 404
    error = response.json()
    assert error['message'] is not None
    assert error['status'] is not None
    assert error['status'] == "404"


def test_template_not_found(client):
    url = 'http://localhost:8088/v1-catalog/templates/xyz:pqr'
    response = requests.get(url)
    assert response.status_code == 404
    error = response.json()
    assert error['message'] is not None
    assert error['status'] is not None
    assert error['status'] == "404"


def test_templatebase_eq_filter(client):
    templates = client.list_template(templateBase_eq='invalid')
    assert len(templates) == 0


def test_templatebase_ne_filter(client):
    templates = client.list_template(templateBase_ne='invalid')
    assert len(templates) > 0


def test_template_files_map(client):
    templates = client.list_template()
    assert len(templates) > 0
    versionUrls = templates[0].versionLinks.values()

    url = versionUrls[0]
    response = requests.get(url)
    assert response.status_code == 200
    resp = response.json()
    assert resp['files'] is not None
    assert resp['files']['rancher-compose.yml'] is not None


def test_template_base(client):
    templates = client.list_template()
    assert len(templates) > 0
    for i in range(len(templates)):
        assert templates[i].templateBase is not None


def test_template_many_versions(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrls = templates[i].versionLinks.values()
                for i in range(len(versionUrls)):
                    version_response = requests.get(versionUrls[i])
                    assert version_response is not 404
                    response_json = version_response.json()
                    docker_compose = response_json.get(unicode('files')) \
                        .get(unicode('docker-compose.yml'))
                    assert docker_compose is not None


def test_template_is_system_flag(client):
    templates = client.list_template()
    assert len(templates) > 0
    for i in range(len(templates)):
        assert templates[i].isSystem is not None


def test_template_minimum_rancher_version_filter(client):
    templates = client.list_template(catalogId='qa-catalog',
                                     minimumRancherVersion_lte='v0.46.0')
    assert len(templates) > 0

    temp = client.list_template(catalogId='qa-catalog',
                                minimumRancherVersion_lte='v0.46.0-dev5-rc1')
    assert len(temp) > 0

    # test to check the minimumRancherVersion_lte is applied to upgradeInfo
    # as well
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrlsMap = templates[i].versionLinks
                if len(versionUrlsMap) > 0:
                    url_to_try = versionUrlsMap.get(unicode('1.0.0'))
                    version_response = requests.get(url_to_try)
                    assert version_response is not 404
                    response_json = version_response.json()
                    upgradeUrls = response_json.\
                        get(unicode('upgradeVersionLinks'))
                    assert upgradeUrls is not None
                    min_version_response = requests.\
                        get(url_to_try +
                            "?minimumRancherVersion_lte=v0.46.0")
                    assert version_response is not 404
                    min_response_json = min_version_response.json()
                    minUpgradeUrls = min_response_json.\
                        get(unicode('upgradeVersionLinks'))
                    assert minUpgradeUrls is not None
                    assert len(upgradeUrls) > len(minUpgradeUrls)


def test_template_maximum_rancher_version_filter(client):
    templates = client.list_template(catalogId='qa-catalog',
                                     maximumRancherVersion_gte='v0.46.0')
    assert len(templates) > 0

    temp = client.list_template(catalogId='qa-catalog',
                                maximumRancherVersion_gte='v0.46.0-dev5-rc1')
    assert len(temp) > 0

    # test to check the maximumRancherVersion_gte is applied to upgradeInfo
    # as well
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrlsMap = templates[i].versionLinks
                if len(versionUrlsMap) > 0:
                    url_to_try = versionUrlsMap.get(unicode('1.0.0'))
                    version_response = requests.get(url_to_try)
                    assert version_response is not 404
                    response_json = version_response.json()
                    upgradeUrls = response_json.\
                        get(unicode('upgradeVersionLinks'))
                    assert upgradeUrls is not None
                    max_version_response = requests.\
                        get(url_to_try +
                            "?maximumRancherVersion_gte=v1.5.0")
                    assert version_response is not 404
                    max_response_json = max_version_response.json()
                    maxUpgradeUrls = max_response_json.\
                        get(unicode('upgradeVersionLinks'))
                    assert maxUpgradeUrls is not None
                    assert len(upgradeUrls) > len(maxUpgradeUrls)


def test_template_upgrade_version_links(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            versionUrlsMap = templates[i].versionLinks
            if len(versionUrlsMap) > 0:
                for key in versionUrlsMap.keys():
                    url_to_try = versionUrlsMap[key]
                    version_response = requests.get(url_to_try)
                    assert version_response is not 404
                    response_json = version_response.json()
                    upgradeUrls = response_json. \
                        get(unicode('upgradeVersionLinks'))
                    if upgradeUrls is not None:
                        for key in upgradeUrls.keys():
                            assert upgradeUrls[key] is not None


def test_template_bindings_property(client):
    templates = client.list_template()
    assert len(templates) > 0
    for i in range(len(templates)):
        versionUrls = templates[i].versionLinks.values()
        for i in range(len(versionUrls)):
            wait_for(
                lambda: requests.get(versionUrls[i]).status_code == 200
            )
            response = requests.get(versionUrls[i])
            assert response.status_code == 200
            resp = response.json()
            assert resp['bindings'] is not None
            for i in range(len(resp['bindings'])):
                services = resp['bindings']['services']
                for service in services:
                    service_map = resp['bindings']['services'][service]
                    assert service_map['labels'] is not None
                    assert service_map['ports'] is not None


def test_template_upgrade_version_links_compare_versions(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].name == 'Out of Order Versions':
                versionUrlsMap = templates[i].versionLinks
        if len(versionUrlsMap) > 0:
            versionsArray = ["1.0.0", "1.0.1", "1.0.2", "1.0.3",
                             "1.0.11", "1.1.0", "1.1.1", "1.2.0", "1.2.1",
                             "2.0.0-alpha1", "2.0.0-alpha2", "2.0.0-beta1",
                             "2.0.0"]
            for key in versionUrlsMap.keys():
                versionIndex = versionsArray.index(key)
                url_to_try = versionUrlsMap[key]
                version_response = requests.get(url_to_try)
                assert version_response is not 404
                response_json = version_response.json()
                upgradeUrls = response_json. \
                    get(unicode('upgradeVersionLinks'))
                assert sorted(versionsArray[versionIndex+1:]) \
                    == sorted(upgradeUrls.keys())
                assert len(upgradeUrls) == len(versionsArray) \
                    - versionIndex - 1


def test_template_upgrade_from(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].name == 'Test Upgrade Links':
                versionUrlsMap = templates[i].versionLinks
        if len(versionUrlsMap) > 0:
            for key in versionUrlsMap.keys():
                url_to_try = versionUrlsMap[key]
                version_response = requests.get(url_to_try)
                assert version_response is not 404
                response_json = version_response.json()
                upgradeUrls = response_json. \
                    get(unicode('upgradeVersionLinks'))
                if key == "1.0.0":
                    assert len(upgradeUrls) == 10


def test_v2_upgrade(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].name == 'version 2 test':
                versionUrlsMap = templates[i].versionLinks
        if len(versionUrlsMap) > 0:
            versionsArray = ["1.0.0", "2.0.0", "3.0.0", "4.0.0"]
            assert sorted(versionUrlsMap.keys()) == versionsArray


def test_upgrade_filters(client):
    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrlsMap = templates[i].versionLinks

    filter = "v2.2.0"
    templates = client.list_template(catalogId='qa-catalog',
                                     maximumRancherVersion_gte=filter,
                                     minimumRancherVersion_lte=filter)
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrlsMap = templates[i].versionLinks
        assert len(versionUrlsMap) == 1
        assert "1.0.14" in versionUrlsMap

    templates = client.list_template(catalogId='qa-catalog')
    if len(templates) > 0:
        for i in range(len(templates)):
            if templates[i].id == unicode('qa-catalog:many-versions'):
                versionUrlsMap = templates[i].versionLinks
        for key in versionUrlsMap.keys():
            if key == "1.0.12":
                url_to_try = versionUrlsMap[key]
                response = requests. \
                    get(url_to_try +
                        '?minimumRancherVersion_lte=v2.2.0&'
                        'maximumRancherVersion_gte=v2.2.0')
                assert response is not 404
                response_json = response.json()
                upgradeUrls = response_json. \
                    get(unicode('upgradeVersionLinks'))
                assert len(upgradeUrls) == 1
                assert "1.0.14" in upgradeUrls
