import pytest
import cattle
import requests


@pytest.fixture
def client():
    url = 'http://localhost:8088/v1-catalog/schemas'
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
